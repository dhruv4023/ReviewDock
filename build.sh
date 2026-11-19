#!/bin/sh
set -e
wails build
mv "build/bin/review-dock" "/home/$USER/reviewdock"
