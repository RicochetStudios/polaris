# Polaris

A Kubernetes operator for deploying dedicated video game servers.

Games available to host can be found in the [registry](https://github.com/RicochetStudios/registry).

## Description

Bringing dedicated video game servers to the Kubernetes ecosystem.

Running game servers in containers is already useful for consistency and efficiency.
This project takes things a step further by allowing the servers to be managed by an orchestrator.

In doing so, we can reap some of the benefits already built into Kubernetes:

- Isolation - servers cannot access other instances
- Self healing - if a failure occurs, servers automatically recover and resume within minutes
- Platform agnostic - runs anywhere Kubernetes can; on any cloud

Not every Kubernetes component can be leveraged however, since game servers aren't compatible with some.
Dedicated servers cannot have replicas, so cannot be highly available.

We believe that the benefits vastly outweigh the could have been's.

## Supported versions

- go version v1.21.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

## Contributing

Rules for contributing to this repository.

We use [VS Code](https://code.visualstudio.com/) as our IDE and [Dev Containers](https://containers.dev/) to standardize our local environments.

### Prerequisites

- [VS Code](https://code.visualstudio.com/download)
- [VS Code Dev Containers](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-containers#installation)

### Steps

1. Press `F1` to open the command palette.
1. Select `Dev Containers: Clone Repository in Container Volume...`.
1. Select or enter the [url for this repository](https://github.com/RicochetStudios/polaris.git).
1. Select the `main` branch.
