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
app.kubernetes.io/version: {{ .Values.image.tag | default .Chart.AppVersion | quote }}
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
Validate required values that must not remain as placeholders.
*/}}
{{- define "hyperfleet-api.validateValues" -}}
{{- $registry := trim (toString .Values.image.registry) -}}
{{- if or (not $registry) (eq $registry "CHANGE_ME") -}}
{{- fail "image.registry must be set to actual container registry via --set image.registry=XXX. See docs/deployment.md for examples" -}}
{{- end -}}
{{- $repository := trim (toString .Values.image.repository) -}}
{{- if or (not $repository) (eq $repository "CHANGE_ME") -}}
{{- fail "image.repository must be set (e.g. --set image.repository=openshift-hyperfleet/hyperfleet-api). See docs/deployment.md for examples" -}}
{{- end -}}
{{- if not (trim (toString .Values.image.tag)) -}}
{{- fail "image.tag must be set (e.g. --set image.tag=abc1234)" -}}
{{- end -}}
{{- end }}

{{/*
Database environment variables (using secretKeyRef - Kubernetes best practice)
*/}}
{{- define "hyperfleet-api.databaseEnvVars" -}}
{{- $secretName := "" }}
{{- if .Values.database.external.enabled }}
{{- $secretName = .Values.database.external.secretName }}
{{- else if .Values.database.postgresql.enabled }}
{{- $secretName = printf "%s-db-secrets" (include "hyperfleet-api.fullname" .) }}
{{- end }}
{{- if $secretName }}
- name: HYPERFLEET_DATABASE_HOST
  valueFrom:
    secretKeyRef:
      name: {{ $secretName }}
      key: db.host
- name: HYPERFLEET_DATABASE_PORT
  valueFrom:
    secretKeyRef:
      name: {{ $secretName }}
      key: db.port
- name: HYPERFLEET_DATABASE_NAME
  valueFrom:
    secretKeyRef:
      name: {{ $secretName }}
      key: db.name
- name: HYPERFLEET_DATABASE_USERNAME
  valueFrom:
    secretKeyRef:
      name: {{ $secretName }}
      key: db.user
- name: HYPERFLEET_DATABASE_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ $secretName }}
      key: db.password
{{- end }}
{{- end }}

