{{/*
Expand the name of the chart.
*/}}
{{- define "kubepilot.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Fully qualified app name.
*/}}
{{- define "kubepilot.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{- define "kubepilot.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "kubepilot.labels" -}}
helm.sh/chart: {{ include "kubepilot.chart" . }}
{{ include "kubepilot.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "kubepilot.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kubepilot.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "kubepilot.backendSelectorLabels" -}}
{{ include "kubepilot.selectorLabels" . }}
app.kubernetes.io/component: backend
{{- end }}

{{- define "kubepilot.frontendSelectorLabels" -}}
{{ include "kubepilot.selectorLabels" . }}
app.kubernetes.io/component: frontend
{{- end }}

{{- define "kubepilot.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "kubepilot.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Name of the Secret holding the JWT signing key, admin password and DB password.
*/}}
{{- define "kubepilot.secretName" -}}
{{- if .Values.secret.existingSecret }}
{{- .Values.secret.existingSecret }}
{{- else }}
{{- include "kubepilot.fullname" . }}
{{- end }}
{{- end }}

{{- define "kubepilot.backendFullname" -}}
{{- printf "%s-backend" (include "kubepilot.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "kubepilot.frontendFullname" -}}
{{- printf "%s-frontend" (include "kubepilot.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Storage driver resolution: an external PostgreSQL always wins over the
storage.driver setting, so enabling it is enough to switch backends.
*/}}
{{- define "kubepilot.storageDriver" -}}
{{- if .Values.externalPostgresql.enabled -}}
postgres
{{- else -}}
{{- .Values.storage.driver -}}
{{- end -}}
{{- end }}

{{- define "kubepilot.cacheDriver" -}}
{{- if .Values.externalRedis.enabled -}}
redis
{{- else -}}
{{- .Values.cache.driver -}}
{{- end -}}
{{- end }}

{{/*
PVC name used for SQLite storage.
*/}}
{{- define "kubepilot.pvcName" -}}
{{- if .Values.persistence.existingClaim }}
{{- .Values.persistence.existingClaim }}
{{- else }}
{{- printf "%s-data" (include "kubepilot.fullname" .) }}
{{- end }}
{{- end }}

{{/*
Whether a PersistentVolumeClaim is needed: only the SQLite driver keeps state on
disk.
*/}}
{{- define "kubepilot.needsPersistence" -}}
{{- if and .Values.persistence.enabled (eq (include "kubepilot.storageDriver" .) "sqlite") -}}
true
{{- end -}}
{{- end }}
