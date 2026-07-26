{{- define "nodeharvest.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "nodeharvest.fullname" -}}
{{- if .Values.fullnameOverride }}{{ .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}{{ include "nodeharvest.name" . }}{{- end }}
{{- end }}

{{- define "nodeharvest.labels" -}}
app.kubernetes.io/name: {{ include "nodeharvest.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "nodeharvest.selectorLabels" -}}
app.kubernetes.io/name: {{ include "nodeharvest.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "nodeharvest.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "nodeharvest.fullname" .) .Values.serviceAccount.name }}
{{- else }}{{ default "default" .Values.serviceAccount.name }}{{- end }}
{{- end }}

{{- define "nodeharvest.configMapName" -}}
{{- if .Values.config.existingConfigMap }}{{ .Values.config.existingConfigMap }}
{{- else }}{{ include "nodeharvest.fullname" . }}-config{{- end }}
{{- end }}
