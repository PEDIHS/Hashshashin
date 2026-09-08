# Hashshashin

Hashshashin is an independently implemented Linux L3 tunnel focused on two modes:

- **full**: upload and download both use the tunnel.
- **direct-return**: upload uses the Iran → Kharej tunnel, while Kharej → Iran return traffic uses the normal Internet route without changing the client endpoint.

The project is intentionally designed as an original core rather than a source copy of Paqet, Backhaul, or BackPack. Their public architectural ideas are used only as references for transport and routing concepts.

> Status: early alpha. The development branch will contain the first testable core, installer and integration tests before merging into `main`.
