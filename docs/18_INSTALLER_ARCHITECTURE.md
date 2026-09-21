# One-Click Installer Architecture (Phase 18)

This document details the mechanics and security of the Application Installer.

## 1. Orchestration
A One-Click install isn't just about extracting files; it requires provisioning a database, a database user, granting privileges, and then linking them to the application.
Instead of doing all of this sequentially inside the Agent (which would break the single-responsibility principle of Agent tasks), the Control Plane acts as the **Orchestrator**:
1. It creates the database natively via the `CreateDatabase` task.
2. It creates the database user via the `CreateDatabaseUser` task.
3. It grants privileges via the `GrantPrivileges` task.
4. Finally, it sends the `InstallApp` task with the fully provisioned DB credentials.

## 2. Zero-Shell Execution & Sandbox
When the Agent handles `InstallApp`:
- It uses `exec.Command` with strict arguments to execute `/usr/bin/curl` and `/usr/bin/tar`.
- It does **not** use `sh -c` to chain commands or resolve globs, which prevents a malicious username from breaking out into a shell.
- It uses a Go `text/template` to generate the `wp-config.php` file, safely interpolating the DB credentials.
- It writes the configuration directly using `os.WriteFile` and enforces correct Linux file ownership using `os.Chown`.

## 3. Extensibility
Currently, the system only supports WordPress (`app_name = "wordpress"`).
However, by keeping the download and templating logic encapsulated in the `operations.HandleInstallApp` function, it is trivial to add support for Joomla, Drupal, or Laravel in the future by simply branching on the `AppName`.
