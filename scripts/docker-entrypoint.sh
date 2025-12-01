#!/bin/sh
set -e

# Take ownership of the /app/data directory.
# This is necessary because when a volume is mounted to /app/data,
# it is owned by root, which prevents the non-root 'gotov' user
# from writing to it.
echo "Taking ownership of /app/data..."
chown -R gotov:gotov /app/data

# Step down from root and execute the main command as the 'gotov' user
exec su-exec gotov "$@"
