apiVersion: v1
kind: Service
metadata:
  name: gateway-service
  namespace: ${NAMESPACE}
  labels:
    app: gateway
    component: edge
${GATEWAY_SERVICE_METADATA_EXTRA}
spec:
  type: ${GATEWAY_SERVICE_TYPE}
${GATEWAY_SERVICE_LOADBALANCER_IP_LINE}
${GATEWAY_SERVICE_EXTERNAL_IPS_BLOCK}
  selector:
    app: gateway
    component: edge
  ports:
    - name: http
      protocol: TCP
      port: 80
      targetPort: 80
    - name: https
      protocol: TCP
      port: 443
      targetPort: 443
    - name: kafka
      protocol: TCP
      port: 9092
      targetPort: 9092
    - name: redis
      protocol: TCP
      port: 6379
      targetPort: 6379
