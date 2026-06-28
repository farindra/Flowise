#!/bin/bash
set -a
source "$(dirname "$0")/.env"
set +a
exec "$(dirname "$0")/go-telegram"
