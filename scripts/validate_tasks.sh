#!/usr/bin/env bash
set -euo pipefail

go test ./server/internal/projectmgmt -run TestValidateTasks
