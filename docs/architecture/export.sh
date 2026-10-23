#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
out="$root/docs/architecture"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

structurizr="${STRUCTURIZR_CLI:-structurizr}"
plantuml="${PLANTUML_JAR:-plantuml.jar}"

"$structurizr" export \
  -workspace "$out/workspace.dsl" \
  -format plantuml/c4plantuml \
  -output "$work"

java -jar "$plantuml" -tpng -o "$work" "$work"/*.puml

cp "$work/structurizr-SystemContext.png" "$out/system-context.png"
cp "$work/structurizr-Containers.png" "$out/containers.png"
cp "$work/structurizr-Components.png" "$out/components.png"
