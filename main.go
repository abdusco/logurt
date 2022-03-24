package main

import (
	"fmt"
	"github.com/golang-jwt/jwt"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"gopkg.in/olahol/melody.v1"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"
)

type logMessage struct {
	Timestamp  string `json:"timestamp"`
	Log        string `json:"log"`
	Kubernetes struct {
		PodName        string            `json:"pod_name"`
		NamespaceName  string            `json:"namespace_name"`
		Labels         map[string]string `json:"labels"`
		ContainerName  string            `json:"container_name"`
		ContainerImage string            `json:"container_image"`
	} `json:"kubernetes"`
}

func randomString(length int) string {
	rand.Seed(time.Now().UnixNano())
	b := make([]byte, length*2)
	rand.Read(b)
	return fmt.Sprintf("%x", b)[2 : length+2]
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	signingKey := os.Getenv("JWT_SIGNING_KEY")
	if signingKey == "" {
		// this key will change on every restart
		signingKey = randomString(64)
		log.Printf("JWT_SIGNING_KEY is not set, using random key: %s", signingKey)
		log.Printf("This will change on every restart and invalidate all previous tokens")
	}

	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Recover())
	e.Use(middleware.Logger())
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		log.Printf("%+v", err)
	}

	m := melody.New()
	m.HandleClose(func(s *melody.Session, _ int, _ string) error {
		log.Printf("Session %+v closed", s.Keys)
		return nil
	})

	e.POST("/sign", func(c echo.Context) error {
		type signRequest struct {
			Namespace string `json:"namespace"`
			Pod       string `json:"pod"`
			Container string `json:"container"`
		}
		req := new(signRequest)
		if err := c.Bind(req); err != nil {
			return err
		}

		signer := jwt.NewWithClaims(jwt.SigningMethodHS256, LogRequestClaims{
			Namespace: req.Namespace,
			Pod:       req.Pod,
			Container: req.Container,
		})
		token, err := signer.SignedString([]byte(signingKey))
		if err != nil {
			return err
		}

		return c.JSON(http.StatusOK, map[string]string{
			"token": token,
		})
	})

	e.POST("/", func(c echo.Context) error {
		var logs []logMessage
		// TODO: migrate to msgpack
		err := c.Bind(&logs)
		if err != nil {
			return err
		}

		for _, it := range logs {
			err := m.BroadcastFilter([]byte(it.Log), func(session *melody.Session) bool {
				namespace := session.Keys["namespace"].(string)
				pod := session.Keys["pod"].(string)
				container := session.Keys["container"].(string)

				if container != "" && it.Kubernetes.ContainerName != container {
					return false
				}

				if pod != "" && it.Kubernetes.PodName != pod {
					return false
				}

				if namespace != "" && it.Kubernetes.NamespaceName != namespace {
					return false
				}

				return true
			})
			if err != nil {
				log.Printf("Error while broadcasting: %+v", err)
				continue
			}

			log.Printf("%s.%s.%s: %s", it.Kubernetes.NamespaceName, it.Kubernetes.PodName, it.Kubernetes.ContainerName, it.Log)
		}

		return c.String(http.StatusOK, "OK")
	})

	e.GET("/logs/ws", func(c echo.Context) error {
		namespace := c.QueryParam("namespace")
		pod := c.QueryParam("pod")
		container := c.QueryParam("container")

		if namespace == "" {
			return badParam(c, "namespace", "namespace is required")
		}

		if container != "" && pod == "" {
			return badParam(c, "pod", "pod is required when container is specified")
		}

		return m.HandleRequestWithKeys(c.Response(), c.Request(), map[string]interface{}{
			"namespace": namespace,
			"pod":       pod,
			"container": container,
		})
	})

	log.Fatal(e.Start(":" + port))
}

func badParam(c echo.Context, param string, message string) error {
	return c.JSON(http.StatusBadRequest, map[string]string{
		"error": message,
		"param": param,
	})
}
