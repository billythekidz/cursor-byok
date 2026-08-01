#!/bin/bash
APP_PATH="/Applications/Cursor Helper.app"

if [ ! -d "$APP_PATH" ]; then
  echo "Please drag Cursor Helper to Applications folder before running this script."
  read -r -p "Press Enter to exit..."
  exit 1
fi

echo "Removing quarantine attribute: $APP_PATH"
xattr -cr "$APP_PATH"
echo "Done! You can now open Cursor Helper."
read -r -p "Press Enter to exit..."
