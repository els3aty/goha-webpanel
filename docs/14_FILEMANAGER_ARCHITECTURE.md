# File Manager Architecture (Phase 14)

This document explains the security mechanisms behind the GohaHost Web File Manager.

## 1. Overview
The File Manager allows end-users to manage files in their home directories directly from the Control Plane browser interface. 
Since the Server Agent executes as `root` (to manage the server), any file operations it performs present an extreme security risk if not strictly bounded.

## 2. The Symlink Sandbox Vulnerability
A common attack vector in control panels is the "Symlink Escape". A malicious user creates a symbolic link in their home directory:
`ln -s /etc/shadow /var/www/hacker_user/public_html/shadow.txt`

If the Control Panel's File Manager naively reads `/var/www/hacker_user/public_html/shadow.txt`, the OS will transparently follow the symlink and return the contents of `/etc/shadow`.

## 3. GohaHost Path Sandbox
To neutralize this, GohaHost implements a custom **Path Sandbox** (`SecureResolvePath` function) entirely in native Go.

**How it works:**
1. The Agent receives the requested relative path (e.g., `public_html/shadow.txt`).
2. It concatenates this with the user's base boundary (e.g., `/var/www/hacker_user/`).
3. It cleans the path to remove directory traversals (`../`).
4. **CRITICAL STEP:** It calls `filepath.EvalSymlinks()`, which asks the OS to resolve the *final physical destination* of the path.
5. The Agent checks if the final physical destination still starts with `/var/www/hacker_user/`.
6. If the path escapes the boundary (like `/etc/shadow`), the operation is instantly aborted with a `403 Forbidden` error.

## 4. Zero-Shell Execution
Instead of passing arguments to `ls`, `cat`, or `rm` via shell execution, the Agent uses pure Go filesystem APIs (`os.ReadDir`, `os.ReadFile`, `os.WriteFile`, `os.RemoveAll`). This eliminates any possibility of command injection via malformed filenames.

## 5. Permission Enforcement
When a file is uploaded or written via the File Manager, the Agent creates the file. Because the Agent is `root`, the file would normally be owned by `root:root`. This would prevent the user's PHP or Node.js apps from writing to or modifying the file.
To fix this securely, the Agent immediately invokes `os.Chown()` to transfer ownership of the file to the user's specific Linux `UID` and `GID`.
