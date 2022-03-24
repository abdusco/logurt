package main

import (
	"github.com/golang-jwt/jwt"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"gopkg.in/olahol/melody.v1"
	"log"
	"net/http"
	"os"
	"strings"
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

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	signingKey := os.Getenv("JWT_SIGNING_KEY")
	ingestionKey := os.Getenv("LOG_INGESTION_KEY")

	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		log.Printf("%+v", err)
	}
	e.Use(middleware.Recover())
	e.Use(middleware.Logger())

	if signingKey != "" {
		e.Use(middleware.JWTWithConfig(middleware.JWTConfig{
			SigningKey:  []byte(signingKey),
			TokenLookup: "header:Authorization:Bearer ,query:token",
			Skipper: func(c echo.Context) bool {
				return !strings.HasPrefix(c.Path(), "/logs")
			},
		}))
	} else {
		log.Printf("JWT_SIGNING_KEY is not set, disabling authentication")
	}

	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c.Path() == "/logs" {
				return next(c)
			}
			if c.Request().Header.Get("Secret") != ingestionKey {
				return c.String(http.StatusUnauthorized, "Secret header is not set or invalid")
			}
			return next(c)
		}
	})

	m := melody.New()
	m.HandleClose(func(s *melody.Session, _ int, _ string) error {
		log.Printf("Session %+v closed", s.Keys)
		return nil
	})

	e.POST("/sign", handleSign(signingKey, func(c echo.Context, token string) string {
		u := c.Request().URL
		u.Path = "/logs"
		u.RawQuery = "token=" + token
		return u.String()
	}))
	e.POST("/", handleIngest(m))
	e.GET("/logs/ws", handleLogStream(m)).Name = "logs"

	log.Printf("Listening on port %s", port)
	log.Fatal(e.Start(":" + port))
}

func handleLogStream(m *melody.Melody) echo.HandlerFunc {
	return func(c echo.Context) error {
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
	}
}

func handleIngest(m *melody.Melody) echo.HandlerFunc {
	return func(c echo.Context) error {
		// TODO: add shared secret authentication
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
	}
}

type SignedUrlGenerator func(c echo.Context, token string) string

func handleSign(signingKey string, generator SignedUrlGenerator) echo.HandlerFunc {
	type signRequest struct {
		Namespace string `json:"namespace"`
		Pod       string `json:"pod"`
		Container string `json:"container"`
	}
	type signResponse struct {
		Url string `json:"url"`
	}

	return func(c echo.Context) error {
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

		signedUrl := generator(c, token)

		return c.JSON(http.StatusOK, signResponse{
			Url: signedUrl,
		})
	}
}

func badParam(c echo.Context, param string, message string) error {
	return c.JSON(http.StatusBadRequest, map[string]string{
		"error": message,
		"param": param,
	})
}
