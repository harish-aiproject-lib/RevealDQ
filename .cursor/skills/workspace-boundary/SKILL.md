---
name: workspace-boundary
description: Enforces that the agent cannot create or delete files outside the AgenticDataGov project folder. Use when creating, deleting, moving, or writing any files, or when the user asks about file system boundaries for this workspace.
---

# Workspace Boundary Policy

You can’t delete or add files outside of `/Users/vedantsubramanian/Documents/Harish/AgenticAI_Projects/AgenticDataGov`.

## Before any filesystem write or delete

1. Resolve the target path to an absolute path.
2. Confirm it is under `/Users/vedantsubramanian/Documents/Harish/AgenticAI_Projects/AgenticDataGov`.
3. If not, **stop** and tell the user the operation is blocked by this policy.
4. Never use `..` or symlink tricks to escape the folder.

## Allowed

- Create, edit, delete, and move files inside the AgenticDataGov tree.
- Read files outside for reference when needed to complete a task.

## Forbidden

- Creating or deleting files outside AgenticDataGov.
- Writing into other projects, `/tmp` project clones meant as workarounds, or Cursor internal skill folders.

## Checklist

```
- [ ] Absolute path computed
- [ ] Path starts with /Users/vedantsubramanian/Documents/Harish/AgenticAI_Projects/AgenticDataGov
- [ ] No parent-directory escape
```
