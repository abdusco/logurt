package main

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"gopkg.in/olahol/melody.v1"
	"log"
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
	e := echo.New()
	e.Use(middleware.Recover())
	e.Use(middleware.Logger())
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		log.Printf("%+v", err)
	}
	e.HideBanner = true

	m := melody.New()
	m.HandleClose(func(s *melody.Session, _ int, _ string) error {
		log.Printf("Session %s closed", s.Request.URL.RawQuery)
		return nil
	})

	e.POST("/", func(c echo.Context) error {
		var logs []logMessage
		err := c.Bind(&logs)
		if err != nil {
			return err
		}

		for _, it := range logs {
			err := m.BroadcastFilter([]byte(it.Log), func(session *melody.Session) bool {
				return session.Request.URL.Query().Get("namespace") == it.Kubernetes.NamespaceName
			})
			if err != nil {
				log.Printf("Error while broadcasting: %+v", err)
				continue
			}

			log.Printf("%s.%s.%s: %s", it.Kubernetes.NamespaceName, it.Kubernetes.PodName, it.Kubernetes.ContainerName, it.Log)
		}

		return c.String(200, "OK")
	})
	e.GET("/logs", func(c echo.Context) error {
		return m.HandleRequest(c.Response(), c.Request())
	})

	log.Fatal(e.Start(":8080"))
}
