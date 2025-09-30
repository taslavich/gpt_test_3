apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: ${GRPC_INGRESS_NAME}
  namespace: exchange
  annotations:
    nginx.ingress.kubernetes.io/backend-protocol: "GRPC"
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
${LETSENCRYPT_CLUSTER_ANNOTATION_LINE}
spec:
  ingressClassName: ${LETSENCRYPT_INGRESS_CLASS}
  tls:
    - hosts:
        - ${GRPC_FQDN}
      secretName: gateway-tls
  rules:
    - host: ${GRPC_FQDN}
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: ${GRPC_SERVICE_NAME}
                port:
                  number: ${GRPC_SERVICE_PORT}
