# logurt

**logurt** is a simple application that ingests logs and serves them in real time over Websocket.
It is designed to be used in a Kubernetes cluster in tandem with Fluentbit / Fluentd.

## Configuration

logurt is configured using environment variables.

- `API_SECRET`: The secret to communicate with the API. This needs to be shared with the applications that call the API.
- `JWT_SIGNING_KEY`: The secret to use for signing JWT tokens. This needs to be sufficiently long and random.
- `LOG_INGESTION_KEY`: The key to use for protecting log ingestion endpoints. This needs to be set in Fluentbit
  configuration as a header.
- `JWT_EXPIRATION_MINUTES`: The number of minutes to set the JWT token expiration to. Defaults to `60`.
- `PORT`: The port to listen on. Defaults to `8080`.

## Endpoints

### Request signing

Available at `/api/sign`.  
Signs a log request and returns a JWT token that is valid for the next `JWT_EXPIRATION_MINUTES` minutes.

```http request
POST /api/sign
Content-Type: application/json
Authorization: Token api_secret_here

{
  "namespace": "myapp", // required
  "pod": "mypod",  // optional
  "container": "mycontainer"  // optional
}
```

returns

```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE2NDgyMDAxNTUsIm5hbWVzcGFjZSI6Im5zIiwicG9kIjoid2ViIiwiY29udGFpbmVyIjoiIn0.-NNQN-zs_vYttHYcMtjecv7id-JHs1fZ6cWr0vj_Zso",
  "url": "/logs/ws?token=eyj..."
}
```

Then the token can be passed in the `Authorization` header of the request as `Authorization: Bearer eyj...`
or in the query string as `?token=eyj...` when connecting to Websocket endpoint.

### Log endpoint

Available at `/logs/ws`.  
This endpoint is used to serve logs over Websocket.
Requires a valid JWT token in the `Authorization` header or query string.

```
GET https://mylogurt.com/logs/ws?token=eyj...
```
```
GET https://mylogurt.com/logs/ws
Authorization: Bearer eyj...
```

This will return the logs in the plaintext format.

### Log ingestion endpoint

Available at `/_ingest/fluentbit`.    
This endpoint is used for ingesting logs as JSON. It is meant to be used by Fluentbit.

Requires a valid token in the `Authorization` header as `Authorization: Token ingestion_key_here`

Expects the log payloads to be in the following JSON format:

```json
{
  "timestamp": "2020-01-01T00:00:00Z",
  "log": "my log message",
  "kubernetes": {
    "namespace": "myapp",
    "pod": "mypod",
    "container": "mycontainer"
  }
}
```

## Usage

You can run logurt locally using the following command. You need to pass in a couple of secrets
to protect the endpoints.

```shell
docker run -it --rm 8080:8080 \
  -e PORT=8081 \
  -e API_SECRET=secret \
  -e JWT_SIGNING_KEY=1234567890qwertyuiopasdfghjklzxcvbnm \
  -e LOG_INGESTION_KEY=log \
  abdusco/logurt
```

## Kubernetes deployment

To deploy logurt to a Kubernetes cluster, you need to have a working Fluentbit/Fluentd setup, deployed as a `DaemonSet`.

### Fluentbit configuration

Fluentbit needs to be configured to ingest Kubernetes logs, and to use the `http` output option.

```
[FILTER]  # this is most likely already set up
    Name  kubernetes
    Match  kube.*
    Kube_URL  https://kubernetes.default.svc:443
    Kube_CA_File  /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
    Kube_Token_File  /var/run/secrets/kubernetes.io/serviceaccount/token
    Kube_Tag_Prefix  kube.var.log.containers.
    Merge_Log  On
    Merge_Log_Key  log_processed
    K8S-Logging.Parser  On
    K8S-Logging.Exclude  Off

[OUTPUT]  # this needs to be added
    Name  http
    Match  kube.*
    Host  logurt.logging.svc.cluster.local
    URI   /_ingest/fluentbit
    Port  8080
    Format  json
    Json_date_key  timestamp
    Json_date_format  iso8601
    Header  Authorization Token $key  # this is the ingestion key passed to logurt
```

## Kubernetes spec

Below is a minimal example of a Kubernetes deployment. Apply it using `kubectl`, `helm`, `terraform` or any other viable
tool.

```yaml
apiVersion: v1
kind: Deployment
metadata:
  name: logurt
  namespace: logging
spec:
  selector:
    matchLabels:
      app: logurt
  template:
    metadata:
      labels:
        app: logurt
    spec:
      containers:
        - name: app
          image: abdusco/logurt:latest
          envFrom:
            - secretRef:
                name: logurt-secrets
---
apiVersion: v1
kind: Service
metadata:
  name: logurt
  namespace: logging
spec:
  type: ClusterIP
  ports:
    - port: 8080
      targetPort: 8080
  selector:
    app: logurt
---
apiVersion: v1
kind: Secret
metadata:
  name: logurt-secrets
  namespace: logging
type: Opaque
stringData:
  # generate secure random strings using `openssl rand -hex 32`
  API_SECRET: api-secret-here
  JWT_SIGNING_KEY: jwt-signing-secret-here
  LOG_INGESTION_KEY: log-ingestion-key-here
```
