#!/usr/bin/env sh

helm repo add jetstack https://charts.jetstack.io
helm repo add rancher-latest https://releases.rancher.com/server-charts/latest
helm repo update
helm upgrade --set crds.enabled=true --install cert-manager jetstack/cert-manager --namespace cert-manager --create-namespace --wait --debug
helm upgrade --install rancher rancher-latest/rancher --namespace cattle-system --create-namespace --set hostname=canary.dev.home.arpa --set bootstrapPassword=changeme --wait --debug
