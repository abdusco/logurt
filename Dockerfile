FROM golang:1.18 as builder

ENV GOOS=linux GOARCH=amd64 CGO_ENABLED=0
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o logurt

FROM scratch
COPY --from=builder /app/logurt /app/logurt
CMD ["/app/logurt"]
EXPOSE 8080