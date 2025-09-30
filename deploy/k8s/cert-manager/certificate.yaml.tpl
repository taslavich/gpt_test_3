apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: gateway-tls
  namespace: exchange
spec:
  secretName: gateway-tls
  issuerRef:
    name: ${LETSENCRYPT_CLUSTER_ISSUER}
    kind: ClusterIssuer
  commonName: ${RTB_DOMAIN}
  dnsNames:
    - ${RTB_DOMAIN}
${TLS_DNS_ADDITIONAL_LINES}
