{{/*
Expand the name of the chart.
*/}}
{{- define "automate.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Fully qualified app name (the release "prefix" used for every resource).
*/}}
{{- define "automate.fullname" -}}
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

{{- define "automate.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels applied to every resource.
*/}}
{{- define "automate.labels" -}}
helm.sh/chart: {{ include "automate.chart" . }}
{{ include "automate.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: automate
{{- end }}

{{/*
Selector labels (release-scoped).
*/}}
{{- define "automate.selectorLabels" -}}
app.kubernetes.io/name: {{ include "automate.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Per-component name and selector labels.
Usage: {{ include "automate.componentSelectorLabels" (dict "root" . "component" "api") }}
*/}}
{{- define "automate.componentSelectorLabels" -}}
{{ include "automate.selectorLabels" .root }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{- define "automate.componentLabels" -}}
{{ include "automate.labels" .root }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{/*
Resource names for the in-cluster dev dependencies.
*/}}
{{- define "automate.postgres.fullname" -}}
{{- printf "%s-postgres" (include "automate.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "automate.temporal.fullname" -}}
{{- printf "%s-temporal" (include "automate.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "automate.api.fullname" -}}
{{- printf "%s-api" (include "automate.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "automate.worker.fullname" -}}
{{- printf "%s-worker" (include "automate.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
ServiceAccount name.
*/}}
{{- define "automate.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "automate.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Effective DATABASE_URL.
- If app.databaseUrl is set explicitly, use it.
- Else if the in-cluster dev Postgres is enabled, form it from that Service.
- Else empty (the app validates and may fail — intended for misconfigured envs).
*/}}
{{- define "automate.databaseUrl" -}}
{{- if .Values.app.databaseUrl -}}
{{- .Values.app.databaseUrl -}}
{{- else if .Values.postgres.enabled -}}
{{- printf "postgres://%s:%s@%s:%d/%s?sslmode=disable" .Values.postgres.auth.username .Values.postgres.auth.password (include "automate.postgres.fullname" .) (int .Values.postgres.service.port) .Values.postgres.auth.database -}}
{{- end -}}
{{- end }}

{{/*
Effective TEMPORAL_HOSTPORT.
- If app.temporalHostPort is set explicitly, use it.
- Else if the in-cluster dev Temporal is enabled, form it from that Service.
- Else empty.
*/}}
{{- define "automate.temporalHostPort" -}}
{{- if .Values.app.temporalHostPort -}}
{{- .Values.app.temporalHostPort -}}
{{- else if .Values.temporal.enabled -}}
{{- printf "%s:%d" (include "automate.temporal.fullname" .) (int .Values.temporal.service.grpcPort) -}}
{{- end -}}
{{- end }}

{{/*
Shared application env block, consumed by both api and worker.
This is the single source of truth for the internal/config env contract.
*/}}
{{- define "automate.appEnv" -}}
- name: APP_ENV
  value: {{ .Values.app.env | quote }}
- name: HTTP_ADDR
  value: {{ .Values.app.httpAddr | quote }}
- name: FILE_STORE
  value: {{ .Values.app.fileStore | quote }}
- name: AUTH_LOCAL_ENABLED
  value: {{ .Values.app.authLocalEnabled | quote }}
- name: DATABASE_URL
  value: {{ include "automate.databaseUrl" . | quote }}
- name: TEMPORAL_HOSTPORT
  value: {{ include "automate.temporalHostPort" . | quote }}
{{- with .Values.app.extraEnv }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Container-level securityContext — hardened per docs/spec/07 §3.
readOnlyRootFilesystem is passed in per component.
Usage: {{ include "automate.containerSecurityContext" (dict "readOnlyRootFilesystem" true) }}
*/}}
{{- define "automate.containerSecurityContext" -}}
allowPrivilegeEscalation: false
readOnlyRootFilesystem: {{ .readOnlyRootFilesystem }}
runAsNonRoot: true
capabilities:
  drop:
    - ALL
seccompProfile:
  type: RuntimeDefault
{{- end }}
