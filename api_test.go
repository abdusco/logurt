package main

import (
	"encoding/json"
	"github.com/labstack/echo/v4"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func Test_handleSign(t *testing.T) {
	mockJwt := func(req LogRequest) (string, error) {
		return "atoken", nil
	}
	mockUrl := func(c echo.Context, token string) (string, error) {
		return "aurl", nil
	}

	tests := []struct {
		name    string
		payload string
		want    map[string]string
	}{
		{
			name:    "missing namespace",
			payload: `{"namespace": "", "pod": "web", "container": "app"}`,
			want: map[string]string{
				"error": "namespace is required",
			},
		},
		{
			name:    "namespace + pod",
			payload: `{"namespace": "ns", "pod": "p"}`,
			want: map[string]string{
				"url":   "aurl",
				"token": "atoken",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := handleSign(signRequestValidator, mockJwt, mockUrl)

			req, _ := http.NewRequest(http.MethodPost, "/", strings.NewReader(tt.payload))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(req, rec)

			_ = handler(c)

			output := make(map[string]string)
			_ = json.Unmarshal(rec.Body.Bytes(), &output)

			if !reflect.DeepEqual(output, tt.want) {
				t.Errorf("handleSign() response = %v, want %v", output, tt.want)
			}
		})
	}
}

func Test_signRequestValidator(t *testing.T) {
	type args struct {
		req *signRequest
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{name: "without namespace", args: args{
			req: &signRequest{
				Namespace: "",
				Pod:       "web",
				Container: "app",
			},
		}, wantErr: true},
		{name: "without pod, with container", args: args{
			req: &signRequest{
				Namespace: "ns",
				Pod:       "",
				Container: "app",
			},
		}, wantErr: true},
		{name: "with namespace, with pod, with container", args: args{
			req: &signRequest{
				Namespace: "ns",
				Pod:       "web",
				Container: "app",
			},
		}, wantErr: false},
		{name: "with namespace, with pod", args: args{
			req: &signRequest{
				Namespace: "ns",
				Pod:       "web",
				Container: "app",
			},
		}, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := signRequestValidator(tt.args.req); (err != nil) != tt.wantErr {
				t.Errorf("signRequestValidator() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
