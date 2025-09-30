apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: ${LETSENCRYPT_CLUSTER_ISSUER}
spec:
  acme:
    email: ${LETSENCRYPT_EMAIL}
    server: ${LETSENCRYPT_SERVER}
    privateKeySecretRef:
      name: ${LETSENCRYPT_CLUSTER_ISSUER}
    solvers:
      - http01:
          ingress:
            class: ${LETSENCRYPT_INGRESS_CLASS}
