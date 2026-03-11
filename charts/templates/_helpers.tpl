{{/*
Expand the name of the chart.
*/}}
{{- define "hyperfleet-api.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "hyperfleet-api.fullname" -}}
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

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "hyperfleet-api.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "hyperfleet-api.labels" -}}
helm.sh/chart: {{ include "hyperfleet-api.chart" . }}
{{ include "hyperfleet-api.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "hyperfleet-api.selectorLabels" -}}
app.kubernetes.io/name: {{ include "hyperfleet-api.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: api
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "hyperfleet-api.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "hyperfleet-api.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Database environment variables (using *_FILE pattern for Viper)
*/}}
{{- define "hyperfleet-api.databaseEnvVars" -}}
{{- if .Values.database.external.enabled }}
- name: HYPERFLEET_DATABASE_HOST_FILE
  value: /app/secrets/database/db.host
- name: HYPERFLEET_DATABASE_PORT_FILE
  value: /app/secrets/database/db.port
- name: HYPERFLEET_DATABASE_NAME_FILE
  value: /app/secrets/database/db.name
- name: HYPERFLEET_DATABASE_USERNAME_FILE
  value: /app/secrets/database/db.user
- name: HYPERFLEET_DATABASE_PASSWORD_FILE
  value: /app/secrets/database/db.password
{{- else if .Values.database.postgresql.enabled }}
- name: HYPERFLEET_DATABASE_HOST_FILE
  value: /app/secrets/database/db.host
- name: HYPERFLEET_DATABASE_PORT_FILE
  value: /app/secrets/database/db.port
- name: HYPERFLEET_DATABASE_NAME_FILE
  value: /app/secrets/database/db.name
- name: HYPERFLEET_DATABASE_USERNAME_FILE
  value: /app/secrets/database/db.user
- name: HYPERFLEET_DATABASE_PASSWORD_FILE
  value: /app/secrets/database/db.password
{{- end }}
{{- end }}

{{/*
Database secret volume mounts
With pgbouncer: app connects to pgbouncer at localhost (using pgbouncer secret)
Without pgbouncer: app connects directly to DB (using real DB or external secret)
*/}}
{{- define "hyperfleet-api.secretVolumeMounts" -}}
{{- if or .Values.database.external.enabled .Values.database.postgresql.enabled }}
- name: database-secrets
  mountPath: /app/secrets/database
  readOnly: true
{{- end }}
{{- end }}

{{/*
Database secret volumes
With pgbouncer: use pgbouncer secret (db.host=localhost, db.port=6432)
Without pgbouncer: use external secret or postgresql secret (real DB connection)
*/}}
{{- define "hyperfleet-api.secretVolumes" -}}
{{- if or .Values.database.external.enabled .Values.database.postgresql.enabled }}
- name: database-secrets
  secret:
    {{- if .Values.database.pgbouncer.enabled }}
    secretName: {{ include "hyperfleet-api.fullname" . }}-db-secrets-pgbouncer
    {{- else if .Values.database.external.enabled }}
    secretName: {{ .Values.database.external.secretName }}
    {{- else }}
    secretName: {{ include "hyperfleet-api.fullname" . }}-db-secrets
    {{- end }}
{{- end }}
{{- end }}
