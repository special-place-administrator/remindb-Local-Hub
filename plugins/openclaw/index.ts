import { definePluginEntry } from "openclaw/plugin-sdk/plugin-entry";

export default definePluginEntry({
    id: "remindb",
    name: "ReminDB-Local-Hub",
    description:
        "Mounts ReminDB-Local-Hub so OpenClaw agents can query and update a compiled view of their workspace.",
    register(_api) {
        // MCP server is registered at the gateway level (openclaw mcp set).
    },
});
