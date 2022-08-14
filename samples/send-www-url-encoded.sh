#!/bin/bash

export TS=$(date +"%s")

cat requests.json.now | sed -e "s@NOW@\"${TS}\"@g" | curl -v http://localhost:9108/metric -H "application/x-www-form-urlencoded" -d @-
