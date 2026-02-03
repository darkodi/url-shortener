## ADR-001: Monolith First Approach

**Date:** 2026-02-03

**Status:** Accepted

### Context
We're building a URL shortener to learn system architecture.

### Decision
Start with a monolithic architecture before distributing.

### Rationale
- Simpler to understand and debug
- Premature optimization is the root of all evil
- We'll extract services when we understand the boundaries

### Consequences
- Single point of failure
- Will need refactoring in later stages
