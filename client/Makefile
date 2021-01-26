export CGO_ENABLED=0

APP_NAME = client
BUILD_DIR = .build

.PHONY: build clean local-env

all: test build run

verify:
	go vet ./...

test: verify
	go test ./...

build:
	go build -ldflags="-s -w" -o $(BUILD_DIR)/$(APP_NAME) .

run:
	env $$(cat $(APP_NAME).env.local) $(BUILD_DIR)/$(APP_NAME)

local-env:
	@grep -ve "^##.*" $(SVC_NAME).env  > $(SVC_NAME).env.local

clean:
	rm -rf $(APP) $(BUILD)