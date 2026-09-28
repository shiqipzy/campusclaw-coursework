FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /campusclaw .

FROM alpine:3.22
RUN addgroup -S app && adduser -S -G app app && mkdir /uploads && chown app:app /uploads
USER app
COPY --from=build /campusclaw /campusclaw
EXPOSE 8081
CMD ["/campusclaw"]
