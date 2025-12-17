# httspy

**httspy** is a lightweight, secure HTTP sink designed to receive and log mirrored traffic. It is intended to be used with the **Gateway API `RequestMirror`** filter or similar traffic shadowing mechanisms (e.g., Envoy's `request_mirror_policy`).

Its primary purpose is to provide **network visibility** and request metadata without impacting the critical path of your live traffic.

## Features

*   **Request Sizing**: Calculates the **total request size** by estimating the request line, headers, and reading the full body size.
*   **Structured Logging**: Groups metadata for easy parsing and analysis.
*   **Automatic Redaction**:
    *   `Authorization`, `Proxy-Authorization`, `Cookie`, and `Set-Cookie` headers are fully redacted (replaced with `[REDACTED]`).
    *   All other headers are captured for full context.
*   **Structured JSON**: Logs are output in structured JSON format for easy parsing.
*   **Zero Response**: Returns `200 OK` but no content, as the response to a mirrored request is ignored by the gateway.

## Usage

httspy is designed to run as a standalone deployment in your cluster.

### Local Development

```bash
go run main.go
# Send a test request
curl -v -H "Authorization: Bearer secret" http://localhost:8080/path
```

### Kubernetes Deployment

Deploy `httspy` to a monitoring namespace (e.g., `monitoring`) and configure your `HTTPRoute` to mirror traffic to it.

#### 1. Deploy httspy

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: httspy
  namespace: monitoring
spec:
  replicas: 1
  selector:
    matchLabels:
      app: httspy
  template:
    metadata:
      labels:
        app: httspy
    spec:
      containers:
        - name: httspy
          image: ghcr.io/jonpulsifer/httspy:latest
          ports:
            - containerPort: 8080
```

*See `kubernetes/httspy.yaml` for a full example including Service and SecurityContext.*

#### 2. Configure Request Mirroring

Add the `requestMirror` filter to your `HTTPRoute`.

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: production-app
  namespace: app-namespace
spec:
  rules:
  - filters:
    - type: RequestMirror
      requestMirror:
        backendRef:
          name: httspy
          namespace: monitoring
          port: 8080
```

#### 3. Cross-Namespace Permissions

If `httspy` is in a different namespace than the route (e.g., `httspy` in `monitoring`, route in `app-namespace`), you must allow the reference using a `ReferenceGrant`.

```yaml
apiVersion: gateway.networking.k8s.io/v1beta1
kind: ReferenceGrant
metadata:
  name: allow-mirror-to-monitoring
  namespace: monitoring
spec:
  from:
  - group: gateway.networking.k8s.io
    kind: HTTPRoute
    namespace: app-namespace
  to:
  - group: ""
    kind: Service
```

## Sample Log Output

```json
{
  "time": "2023-10-27T10:00:00Z",
  "level": "INFO",
  "msg": "request_mirrored",
  "flow": {
    "total_request_size": 1234,
    "header_size": 450,
    "body_size": 784,
    "latency": 150000,
    "client_ip": "10.0.0.1:12345"
  },
  "http": {
    "method": "POST",
    "host": "example.com",
    "uri": "/api/v1/resource",
    "proto": "HTTP/1.1",
    "user_agent": "curl/7.68.0"
  },
  "headers": {
    "authorization": "[REDACTED]",
    "content-type": "application/json",
    "cookie": "[REDACTED]",
    "x-request-id": "req-123-abc"
  }
}
```
