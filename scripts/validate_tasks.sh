#!/usr/bin/env bash
set -euo pipefail

go test ./internal/projectmgmt -run TestValidateTasks
