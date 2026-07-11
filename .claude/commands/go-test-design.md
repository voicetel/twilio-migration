# Go Test Design

You are designing tests for Go code in this repository.

Interactive requirements:

1. Load local MCP state and bootstrap health.
2. Load or create task items for test design and implementation.
3. Read the changed code, existing tests, package documentation, and relevant docs.
4. Identify all branches, edge cases, error paths, concurrency paths, and boundary conditions.
5. Ask focused interactive questions when expected behavior is not defined.
6. Produce a test plan that can reach today's ratchet coverage floor (target 100%) for changed and affected code.

Output:

- Packages/files under test.
- Existing coverage gaps.
- Proposed table-driven tests.
- Error and edge cases.
- Interactive questions, if any.
