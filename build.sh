#!/bin/sh
set -e
wails build
mv "build/bin/review-dock" "/home/$USER/reviewdock"
# Headless knowledge CLI (cron / systemd timer); shares the app's database.
go build -o "/home/$USER/review-dock-kb" ./cmd/review-dock-kb
