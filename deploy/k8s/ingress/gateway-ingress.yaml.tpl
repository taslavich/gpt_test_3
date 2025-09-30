apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: gateway-ingress
  namespace: exchange
  annotations:
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
    nginx.ingress.kubernetes.io/backend-protocol: "HTTP"
${LETSENCRYPT_CLUSTER_ANNOTATION_LINE}
spec:
  ingressClassName: ${LETSENCRYPT_INGRESS_CLASS}
  tls:
    - hosts:
        - ${RTB_DOMAIN}
      secretName: gateway-tls
  rules:
    - host: ${RTB_DOMAIN}
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: gateway-service
                port:
                  number: 80
