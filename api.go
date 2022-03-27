package main

import (
	"fmt"
	"github.com/golang-jwt/jwt"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"gopkg.in/olahol/melody.v1"
	"log"
	"net/http"
	"time"
)

type AppConfig struct {
	Port               int
	ApiSecret          string
	JwtSigningKey      string
	JwtLifetimeMinutes int
}

type App struct {
	config AppConfig
	melody *melody.Melody
	e      *echo.Echo
}

func (a *App) Run() {
	log.Fatalln(a.e.Start(fmt.Sprintf(":%d", a.config.Port)))
}

func NewApp(config AppConfig) *App {
	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		log.Printf("%+v", err)
	}
	e.Use(middleware.Recover())
	e.Use(middleware.Logger())

	m := melody.New()
	m.HandleClose(func(s *melody.Session, _ int, _ string) error {
		log.Printf("Session %+v closed", s.Keys)
		return nil
	})

	authMiddleware := middleware.KeyAuthWithConfig(middleware.KeyAuthConfig{
		KeyLookup:  "header:Authorization",
		AuthScheme: "Token",
		Validator: func(auth string, c echo.Context) (bool, error) {
			return auth == config.ApiSecret, nil
		},
	})

	api := e.Group("/api")
	api.Use(authMiddleware)
	api.POST("/sign", handleSign(
		signRequestValidator,
		func(req LogRequest) (string, error) {
			// TODO: inject clock
			jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, LogRequestClaims{
				StandardClaims: jwt.StandardClaims{
					ExpiresAt: time.Now().Add(time.Minute * time.Duration(config.JwtLifetimeMinutes)).Unix(),
				},
				LogRequest: req,
			})
			return jwtToken.SignedString([]byte(config.JwtSigningKey))
		},
		func(c echo.Context, token string) (string, error) {
			u := c.Request().URL
			u.Path = "/logs/ws"
			u.RawQuery = "token=" + token
			return u.String(), nil
		}),
	)

	ingest := e.Group("/_ingest")
	ingest.Use(authMiddleware)
	ingest.POST("/fluentbit", handleIngestFluentbit(
		func(message *fluentbitLogMessage) error {
			filter := logFilterFactory(message)
			// TODO: dispatch in a goroutine?
			return m.BroadcastFilter([]byte(message.Log), filter)
		},
	))

	public := e.Group("")
	public.Use(middleware.JWTWithConfig(middleware.JWTConfig{
		SigningKey:  []byte(config.JwtSigningKey),
		TokenLookup: "header:Authorization,query:token",
		AuthScheme:  "Bearer",
		ContextKey:  logRequestClaimsKey,
		Claims:      &LogRequestClaims{},
	}))
	public.GET("/logs/ws", handleLogStream(m))

	return &App{
		config: config,
		melody: m,
		e:      e,
	}
}

const (
	logRequestClaimsKey = "logRequestClaims"
)

func handleLogStream(m *melody.Melody) echo.HandlerFunc {
	return func(c echo.Context) error {
		req := c.Get(logRequestClaimsKey).(*LogRequestClaims)

		return m.HandleRequestWithKeys(c.Response(), c.Request(), map[string]interface{}{
			"req": req.LogRequest,
		})
	}
}

func logFilterFactory(message *fluentbitLogMessage) func(s *melody.Session) bool {
	return func(session *melody.Session) bool {
		req := session.Keys["req"].(LogRequest)

		if req.Namespace != message.Kubernetes.Namespace {
			return false
		}

		if req.Pod != "" && req.Pod != message.Kubernetes.Pod {
			return false
		}

		if req.Container != "" && message.Kubernetes.Container != req.Container {
			return false
		}

		for k, requestedVal := range req.Labels {
			messageVal, ok := message.Kubernetes.Labels[k]
			if !ok || requestedVal != messageVal {
				return false
			}
		}

		return true
	}
}

type fluentbitLogMessage struct {
	Timestamp  string `json:"timestamp"`
	Log        string `json:"log"`
	Kubernetes struct {
		Pod            string            `json:"pod_name"`
		Namespace      string            `json:"namespace_name"`
		Labels         map[string]string `json:"labels"`
		Container      string            `json:"container_name"`
		ContainerImage string            `json:"container_image"`
	} `json:"kubernetes"`
}
type LogDispatcher func(message *fluentbitLogMessage) error

func handleIngestFluentbit(dispatch LogDispatcher) echo.HandlerFunc {
	return func(c echo.Context) error {
		// TODO: add shared secret authentication
		var logs []fluentbitLogMessage
		// TODO: migrate to msgpack
		err := c.Bind(&logs)
		if err != nil {
			return err
		}

		for _, it := range logs {
			err := dispatch(&it)
			if err != nil {
				log.Printf("Error while dispatching logs: %+v", err)
				continue
			}

			log.Printf("%s.%s.%s: %s", it.Kubernetes.Namespace, it.Kubernetes.Pod, it.Kubernetes.Container, it.Log)
		}

		return c.String(http.StatusOK, "OK")
	}
}

// SignRequestValidator is a function that returns a signed URL.
type SignRequestValidator func(request *signRequest) error

// SignedUrlBuilder is a function that returns a signed URL.
type SignedUrlBuilder func(c echo.Context, token string) (string, error)

// JwtSigner Returns a JWT token to sign the given request
type JwtSigner func(req LogRequest) (string, error)

type signRequest struct {
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	Container string `json:"container"`
}
type signResponse struct {
	Token     string `json:"token"`
	SignedUrl string `json:"url"`
}

type LogRequest struct {
	Namespace string            `json:"ns"`
	Pod       string            `json:"pod"`
	Container string            `json:"ctn"`
	Labels    map[string]string `json:"lbl"`
}

type LogRequestClaims struct {
	jwt.StandardClaims
	LogRequest
}

func signRequestValidator(req *signRequest) error {
	if req.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if req.Container != "" && req.Pod == "" {
		return fmt.Errorf("pod is required when container is specified")
	}
	return nil
}

func handleSign(validator SignRequestValidator, signJwt JwtSigner, buildUrl SignedUrlBuilder) echo.HandlerFunc {
	return func(c echo.Context) error {
		req := new(signRequest)
		if err := c.Bind(req); err != nil {
			return err
		}

		if err := validator(req); err != nil {
			return badRequest(c, err.Error())
		}

		token, err := signJwt(LogRequest{
			Namespace: req.Namespace,
			Pod:       req.Pod,
			Container: req.Container,
		})
		if err != nil {
			return err
		}

		signedUrl, err := buildUrl(c, token)
		if err != nil {
			return err
		}

		return c.JSON(http.StatusOK, signResponse{
			Token:     token,
			SignedUrl: signedUrl,
		})
	}
}

func badRequest(c echo.Context, message string) error {
	return c.JSON(http.StatusBadRequest, map[string]string{
		"error": message,
	})
}
