#!/bin/bash

# Function to generate client certificate with username extension
generate_client_cert() {
    local username=$1
    local filename=$2

    # Generate client private key and CSR
    openssl ecparam -name prime256v1 -genkey -noout -out ${filename}.key
    openssl req -new -key ${filename}.key -out ${filename}.csr -subj "/CN=${username}"

    # Create a temporary extension file for the client
    cat > ${filename}_ext.cnf <<EOF
[usr_cert]
subjectAltName=DNS:localhost,DNS:host.docker.internal,IP:127.0.0.1
keyUsage=critical,digitalSignature,keyEncipherment
extendedKeyUsage=clientAuth

[usr_cert]
1.2.3.4.5.6.7.8=ASN1:UTF8String:${username}
EOF

    # Create client certificate with custom extension
    openssl x509 -req -in ${filename}.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out ${filename}.crt -days 825 -sha256 -extfile ${filename}_ext.cnf -extensions usr_cert

    # Clean up CSR and temporary extension file
    rm ca.srl ${filename}.csr ${filename}_ext.cnf
}

# Check if the correct number of arguments are provided
if [ "$#" -ne 2 ]; then
    echo "Usage: $0 <username> <filename>"
    exit 1
fi

# Ensure the 'certs' directory exists
cd certs

# Generate client certificate with provided arguments
generate_client_cert $1 $2

echo "Client certificate and keys for '$1' have been generated as '$2' in the 'certs' directory."
