#!/bin/bash

echo "hello"
go get -d ./...
env GOOS=linux GOARCH=amd64 go build -o helm-ssm-linux-amd64 main.go
env GOOS=darwin GOARCH=amd64 go build -o helm-ssm-darwin-amd64 main.go
