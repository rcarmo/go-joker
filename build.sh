#!/usr/bin/env bash

go generate ./... && go vet ./... && go build -o joker ./cmd/joker
