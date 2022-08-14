#!/bin/bash

export TS=$(date +"%s")

cat requests.json.now | sed -e "s@NOW@\"${TS}\"@g" | curl --proxy http://localhost:9999 -v http://localhost:9111/metric -H "Content-Type: application/json" -d @-
