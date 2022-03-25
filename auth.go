package main

import "github.com/golang-jwt/jwt"

type LogRequest struct {
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	Container string `json:"container"`
}

type LogRequestClaims struct {
	jwt.StandardClaims
	LogRequest
}
