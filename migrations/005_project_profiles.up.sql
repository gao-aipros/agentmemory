-- AgentMemory v2 — Project Profiles Schema
-- Stores auto-learned project knowledge: top concepts, files, conventions,
-- and common errors discovered during sessions within a project.

-- ===========================================================================
-- Project Profiles
-- ===========================================================================
CREATE TABLE IF NOT EXISTS project_profiles (
    project_slug TEXT PRIMARY KEY,
    top_concepts JSONB NOT NULL DEFAULT '[]',
    top_files JSONB NOT NULL DEFAULT '[]',
    conventions TEXT[] NOT NULL DEFAULT '{}',
    common_errors JSONB NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
