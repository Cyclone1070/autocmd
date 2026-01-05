# 1. Package Design

**Goal**: High cohesion. Code related to a feature are placed close together, usually in the same package.

*   **Package Naming**: Package structure and naming should provide enough context to understand the content and purpose of each package.
    *   **Why**: Clear names enable discoverability and prevent packages from becoming dumping grounds. Generic names lead to junk drawers grouping unrelated logic.
    *   **Guideline**: Names like `helper/`, `service/`, or `util/` are acceptable as parent directories when their children and/or parent have specific, descriptive names.

> [!NOTE]
> **Example**: Acceptable Structure
>
> ```text
> internal/tool/
>   ├── helper/
>   │   ├── pagination/   # Specific: handles offset/limit logic
>   │   └── content/      # Specific: binary detection
>   └── service/
>       ├── fs/           # Specific: filesystem operations
>       └── executor/     # Specific: command execution
> ```
>
> The parent directories (`helper/`, `service/`) provide organizational context, while the actual packages have descriptive names.

> [!NOTE]
> **Single-File Directories Are Acceptable**
>
> When extracting shared code to prevent circular dependencies, a directory with one file is fine. Correct structure matters more than file count.
