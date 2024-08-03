#!/bin/bash

# Set up directory for certificates
mkdir -p certs
cd certs

# Generate CA private key and certificate
openssl ecparam -name prime256v1 -genkey -noout -out ca.key
openssl req -x509 -new -nodes -key ca.key -sha256 -days 1825 -out ca.crt -subj "/CN=JobWorkerCA"

# Generate server private key and CSR
openssl ecparam -name prime256v1 -genkey -noout -out server.key
openssl req -new -key server.key -out server.csr -subj "/CN=jobworker-server"

# Create a temporary extension file for the server
cat > server_ext.cnf <<EOF
subjectAltName=DNS:jobworker-server,DNS:localhost,DNS:host.docker.internal,IP:127.0.0.1
keyUsage=critical,digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
EOF

# Create server certificate
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out server.crt -days 825 -sha256 -extfile server_ext.cnf

# Clean up
rm ca.srl server.csr server_ext.cnf

echo "Certificates and keys have been generated in the 'certs' directory."