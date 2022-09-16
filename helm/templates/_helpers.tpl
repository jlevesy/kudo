{{/*
Expand the name of the chart.
*/}}
{{- define "helm.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "helm.fullname" -}}
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
{{- define "helm.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "helm.labels" -}}
helm.sh/chart: {{ include "helm.chart" . }}
{{ include "helm.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "helm.selectorLabels" -}}
app.kubernetes.io/name: {{ include "helm.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "helm.serviceAccountName" -}}
{{- default (include "helm.fullname" .) .Values.serviceAccount.name }}
{{- end }}

{{/*
Create the name of the cert secret to use
*/}}
{{- define "helm.certSecretName" -}}
{{ include "helm.fullname" . }}-cert
{{- end }}

{{/*
Create the name of the config configmap to use
*/}}
{{- define "helm.configConfigMapName" -}}
{{ include "helm.fullname" . }}-config
{{- end }}

{{/*
Returns the name of the image according to values.
Allow to override standard repository / tag by a full ref
Usefull when passing down a ko reference for developnent
*/}}
{{- define "helm.imageName" -}}
{{- if .Values.image.devRef }}
{{- .Values.image.devRef }}
{{- else }}
{{- .Values.image.repository }}:{{- .Values.image.tag | default .Chart.AppVersion }}
{{- end }}
{{- end }}
