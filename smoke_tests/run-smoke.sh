#! /bin/bash

PORT=3232
export SLACK_API_BASE_URL="http://localhost:$PORT/api"
export SLACK_BOT_TOKEN="xoxb-localslack-test-token"

# optional tuning
export LOCALSLACK_RATE_MAX_ATTEMPTS=40
export LOCALSLACK_RATE_MAX_SLEEP=3

python3 test_localslack_basics_and_ratelimit.py
