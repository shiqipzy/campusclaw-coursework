FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY mcp-server ./mcp-server
RUN CGO_ENABLED=0 go build -trimpath -o /mcp-demo ./mcp-server
FROM alpine:3.22
RUN addgroup -S app && adduser -S -G app app
USER app
COPY --from=build /mcp-demo /mcp-demo
EXPOSE 8090
CMD ["/mcp-demo"]
