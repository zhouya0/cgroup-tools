all:
	GO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o=cgroup-tools cmd/main.go