{{- define "nodeharvest.secretEnv" -}}
- name: NODE_HARVEST_DATABASE_URL
  valueFrom: {secretKeyRef: {name: {{ .Values.secrets.existingSecret }}, key: {{ .Values.secrets.databaseURLKey }}}}
- name: NODE_HARVEST_REDIS_URL
  valueFrom: {secretKeyRef: {name: {{ .Values.secrets.existingSecret }}, key: {{ .Values.secrets.redisURLKey }}}}
- name: NODE_HARVEST_TOKEN
  valueFrom: {secretKeyRef: {name: {{ .Values.secrets.existingSecret }}, key: {{ .Values.secrets.subscriptionTokenKey }}}}
- name: NODE_HARVEST_ADMIN_TOKEN
  valueFrom: {secretKeyRef: {name: {{ .Values.secrets.existingSecret }}, key: {{ .Values.secrets.adminTokenKey }}}}
- name: NODE_HARVEST_SESSION_SECRET
  valueFrom: {secretKeyRef: {name: {{ .Values.secrets.existingSecret }}, key: {{ .Values.secrets.sessionSecretKey }}}}
- name: NODE_HARVEST_OIDC_CLIENT_SECRET
  valueFrom: {secretKeyRef: {name: {{ .Values.secrets.existingSecret }}, key: {{ .Values.secrets.oidcClientSecretKey }}, optional: true}}
- name: NODE_HARVEST_ALERT_WEBHOOK_SECRET
  valueFrom: {secretKeyRef: {name: {{ .Values.secrets.existingSecret }}, key: {{ .Values.secrets.alertWebhookSecretKey }}, optional: true}}
- name: NODE_HARVEST_OBJECT_STORE_ACCESS_KEY
  valueFrom: {secretKeyRef: {name: {{ .Values.secrets.existingSecret }}, key: {{ .Values.secrets.objectStoreAccessKeyKey }}, optional: true}}
- name: NODE_HARVEST_OBJECT_STORE_SECRET_KEY
  valueFrom: {secretKeyRef: {name: {{ .Values.secrets.existingSecret }}, key: {{ .Values.secrets.objectStoreSecretKeyKey }}, optional: true}}
{{- end }}
