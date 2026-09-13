# Manifold Agent — Product Boundary and Naming

> Status: P0 naming and architecture decision

**Manifold Agent** is the product-level AI-native software delivery platform. **Manifold Agent Native Loop** is its engineering lifecycle/orchestration architecture. **Multica** is the current workforce/control-plane foundation; Pi, Codex, Claude Code and similar tools are replaceable Agent harnesses.

```text
Manifold Agent
└── Manifold Agent Native Loop
    └── Multica
        ├── Workspace / Project / Issue
        ├── Agent / Squad / Skill
        ├── Task / Run
        └── Runtime / Daemon
            └── Pi / Codex / Claude Code / ...
                └── GPT / Claude / DeepSeek / ...
```

Naming rule:

- `Manifold Agent` = product
- `Manifold Agent Native Loop` = lifecycle methodology/orchestration architecture
- `MNL` = internal implementation/work-item prefix
- `Backend Agent`, `Frontend Agent`, `Test Agent`, etc. = specialist workers
- Multica `Agent` = existing generic workforce entity; do not rename it for branding

P0 remains inside the Multica fork. Do not create a separate `manifold-agent-server` merely for branding. Consider extraction only when Manifold Agent must orchestrate multiple independent Multica installations, support peer workforce backends, or own an independently deployable enterprise control plane/Context Graph.

Recommended positioning:

> **Manifold Agent is an AI-native software delivery platform that coordinates humans and specialized AI agents across the complete engineering lifecycle, from product intent to production feedback.**

中文：

> **Manifold Agent 是面向研发全生命周期的 AI Agent 原生协作与交付平台。**
