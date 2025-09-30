#!/bin/sh
set -e

echo '🔐 Checking SSL certificates...'
      
if [ ! -f /etc/ssl/fullchain.pem ]; then
    echo '❌ No SSL certificates found. Obtaining certificates...'
        
    mkdir -p /etc/ssl
        
    if certbot certonly --standalone \
        -d twinbidexchange.com \
        --non-interactive \
        --agree-tos \
        -m twinbid@twinbidexchange.com \
        --preferred-challenges http; then
        
        cp /etc/letsencrypt/live/twinbidexchange.com/fullchain.pem /etc/ssl/
        cp /etc/letsencrypt/live/twinbidexchange.com/privkey.pem /etc/ssl/
        echo '✅ SSL certificates copied to /etc/ssl/'
    else
        echo '⚠️ Failed to obtain SSL certificates, continuing without HTTPS'
    fi
else
    echo '✅ SSL certificates already exist in /etc/ssl/'
fi
      
echo '🚀 Starting Nginx...'
exec nginx -g 'daemon off;'