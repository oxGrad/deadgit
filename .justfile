import 'secret.just'

_default:
  @just --choose

run:
  BC_AZURE_DEVOPS_PAT=$BC_AZURE_DEVOPS_PAT go run main.go

scan:
  BC_AZURE_DEVOPS_PAT=$BC_AZURE_DEVOPS_PAT go run main.go scan

scan-debug:
  DB_DEBUG=true BC_AZURE_DEVOPS_PAT=$BC_AZURE_DEVOPS_PAT go run main.go scan

add-ota-bc:
  BC_AZURE_DEVOPS_PAT=$BC_AZURE_DEVOPS_PAT go run main.go org add

test:
  echo $BC_AZURE_DEVOPS_PAT
