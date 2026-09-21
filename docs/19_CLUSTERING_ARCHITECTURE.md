# High Availability (HA) & Clustering Architecture (Phase 19)

This document details how GohaHost achieves enterprise-grade load balancing.

## 1. Node Roles
In a clustered setup, nodes are not identical. We introduced a `role` field to the `nodes` table:
- **`compute`**: The traditional server that runs PHP/Node/Python and stores website files.
- **`loadbalancer`**: A specialized server running only Nginx, designed to accept incoming public traffic and securely distribute it to the Compute nodes.

## 2. Cluster Entity
A `Cluster` groups multiple nodes together. For example, "EU Web Cluster 01". 
A typical highly-available cluster consists of 1 Loadbalancer node and 2+ Compute nodes.

## 3. Dynamic Load Balancer Orchestration
When a cluster is modified or a new VirtualHost is provisioned across a cluster, the Control Plane orchestrates the Load Balancer setup:
1. It queries the DB for all `compute` nodes in the cluster and extracts their IP Addresses.
2. It identifies the `loadbalancer` node for the cluster.
3. It sends an `OpConfigureLoadBalancer` task to the LB Node, containing the domain and the list of Compute IPs.

## 4. Zero-Shell Upstream Generation
The Agent on the Load Balancer uses Go's `text/template` to generate an Nginx `upstream` configuration dynamically.

```nginx
upstream cluster_example.com {
    server 10.0.0.1:80 max_fails=3 fail_timeout=30s;
    server 10.0.0.2:80 max_fails=3 fail_timeout=30s;
}
```

This occurs entirely in the Go binary's memory, ensuring that no malicious domains or fake IP addresses can break out into a bash shell.
The Agent then securely reloads Nginx, instantly enabling Round-Robin traffic distribution.
