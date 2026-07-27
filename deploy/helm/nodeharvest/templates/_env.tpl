{{- define "nodeharvest.secretEnv" -}}
- name: NODE_HARVEST_DATABASE_URL
  valueFrom: {secretKeyRef: {name: {{ .Values.secrets.existingSecret }}, key: {{ .Values.secrets.databaseURLKey }}}}
- name: NODE_HARVEST_REDIS_URL
  valueFrom: {secretKeyRef: {name: {{ .Values.secrets.existingSecret }}, key: {{ .Values.secrets.redisURLKey }}}}
- name: NODE_HARVEST_TOKEN
  valueFrom: {secretKeyRef: {name: {{ .Values.secrets.existingSecret }}, key: {{ .Values.secrets.subscriptionTokenKey }}}}
- name: NODE_HARVEST_LOCAL_AUTH
  value: "1"
- name: NODE_HARVEST_BOOTSTRAP_PASSWORD_HASH
  valueFrom: {secretKeyRef: {name: {{ .Values.secrets.existingSecret }}, key: {{ .Values.secrets.bootstrapPasswordHashKey }}}}
- name: NODE_HARVEST_SESSION_SECRET
  valueFrom: {secretKeyRef: {name: {{ .Values.secrets.existingSecret }}, key: {{ .Values.secrets.sessionSecretKey }}}}
- name: NODE_HARVEST_ALERT_WEBHOOK_SECRET
  valueFrom: {secretKeyRef: {name: {{ .Values.secrets.existingSecret }}, key: {{ .Values.secrets.alertWebhookSecretKey }}, optional: true}}
- name: NODE_HARVEST_OBJECT_STORE_ACCESS_KEY
  valueFrom: {secretKeyRef: {name: {{ .Values.secrets.existingSecret }}, key: {{ .Values.secrets.objectStoreAccessKeyKey }}, optional: true}}
- name: NODE_HARVEST_OBJECT_STORE_SECRET_KEY
  valueFrom: {secretKeyRef: {name: {{ .Values.secrets.existingSecret }}, key: {{ .Values.secrets.objectStoreSecretKeyKey }}, optional: true}}
{{- end }}
