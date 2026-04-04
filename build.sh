#!/bin/bash
set -e

echo "Building Next.js web UI..."
cd web
npm install --silent
npm run build
cd ..

echo "Copying static files..."
rm -rf internal/web/static
cp -r web/out internal/web/static

echo "Building Go binary..."
go build -o glaw ./cmd/glaw/

echo "Done! Run ./glaw serve to start the web UI."
