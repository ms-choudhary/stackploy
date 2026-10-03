FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download && go install github.com/a-h/templ/cmd/templ@$(go list -m -f '{{.Version}}' github.com/a-h/templ)
COPY . .
RUN templ generate && CGO_ENABLED=0 go build -ldflags="-s -w" -o /stackploy .

FROM alpine:3
RUN apk add --no-cache ca-certificates git openssh-client docker-cli docker-cli-compose \
	&& printf 'Host *\n\tStrictHostKeyChecking accept-new\n\tUserKnownHostsFile /data/known_hosts\n' >> /etc/ssh/ssh_config
COPY --from=build /stackploy /usr/local/bin/stackploy
ENV DATA_DIR=/data ADDR=:8080
VOLUME /data
EXPOSE 8080
CMD ["stackploy"]
