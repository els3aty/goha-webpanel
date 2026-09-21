-- 017_create_runtimes.sql

ALTER TABLE virtual_hosts 
ADD COLUMN runtime_type VARCHAR(50) NOT NULL DEFAULT 'php',
ADD COLUMN runtime_port INTEGER;

-- Optional constraint to ensure port is provided if not PHP
ALTER TABLE virtual_hosts
ADD CONSTRAINT chk_runtime_port 
CHECK (runtime_type = 'php' OR runtime_port IS NOT NULL);
