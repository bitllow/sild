import type { EffortLevel, HookCallback, McpSdkServerConfigWithInstance, Options } from '@anthropic-ai/claude-agent-sdk';
import { decide, type Decision } from './guard.ts';

export const BUILTIN_TOOLS = ['Read', 'Grep', 'Glob'];

// Belt to `tools`: these are removed from context even if a default adds them back.
export const REMOVED_TOOLS = [
  'Bash', 'Write', 'Edit', 'MultiEdit', 'NotebookEdit', 'WebFetch', 'WebSearch',
  'Agent', 'Task', 'Skill', 'ToolSearch', 'TodoWrite', 'ExitPlanMode', 'KillShell', 'BashOutput',
];

// Built-in plugins load regardless of settingSources; agents-md would read the checkout's AGENTS.md.
// Ids change with the CLI version; an upgrade can add one that must be turned off here.
export const BUILTIN_PLUGINS_OFF: Record<string, boolean> = {
  'cc-plugin-agents-md@builtin': false,
  'cc-plugin-plugin-authoring@builtin': false,
};

export type SessionSpec = {
  workDir: string;
  srcDir: string;
  // CLAUDE_CONFIG_DIR. Must sit outside workDir so the session cannot read its own transcripts.
  configDir: string;
  sessionId: string;
  resume: boolean;
  systemPrompt: string;
  mcpServers: Record<string, McpSdkServerConfigWithInstance>;
  // Fully qualified names (mcp__<server>__<tool>) the session may call.
  mcpTools: string[];
  // Model credentials and transport for the CLI; nothing else reaches it.
  env: Record<string, string>;
  maxTurns: number;
  maxBudgetUsd: number;
  model?: string;
  effort?: EffortLevel;
  spawn?: Options['spawnClaudeCodeProcess'];
  abortController?: AbortController;
  onDecision?: (tool: string, input: unknown, decision: Decision) => void;
  // Asked after each batch of tool calls; true ends the turn there.
  endTurn?: () => boolean;
};

export function sessionOptions(spec: SessionSpec): Options {
  const tools = new Set([...BUILTIN_TOOLS, ...spec.mcpTools]);
  const guard: HookCallback = async (input) => {
    if (input.hook_event_name !== 'PreToolUse') return {};
    const toolInput = (input.tool_input ?? {}) as Record<string, unknown>;
    const decision = decide({ tools, roots: [spec.srcDir, spec.workDir], cwd: spec.workDir }, input.tool_name, toolInput);
    spec.onDecision?.(input.tool_name, toolInput, decision);
    if (decision.allow) return {};
    return {
      hookSpecificOutput: {
        hookEventName: 'PreToolUse',
        permissionDecision: 'deny',
        permissionDecisionReason: decision.reason,
      },
    };
  };

  // The model otherwise keeps answering within one turn until it writes plain text.
  const stop: HookCallback = async () => (spec.endTurn?.() ? { continue: false, stopReason: 'answered' } : {});

  return {
    cwd: spec.workDir,
    additionalDirectories: [spec.srcDir],
    ...(spec.resume ? { resume: spec.sessionId } : { sessionId: spec.sessionId }),
    systemPrompt: spec.systemPrompt,
    model: spec.model,
    ...(spec.effort && { effort: spec.effort }),

    settingSources: [],
    strictMcpConfig: true,
    skills: [],
    settings: { autoMemoryEnabled: false, enabledPlugins: BUILTIN_PLUGINS_OFF },
    verbatimPrompts: true,

    tools: BUILTIN_TOOLS,
    disallowedTools: REMOVED_TOOLS,
    allowedTools: [...tools],
    permissionMode: 'dontAsk',
    permissionPrompts: 'none',
    mcpServers: spec.mcpServers,
    hooks: { PreToolUse: [{ hooks: [guard] }], PostToolBatch: [{ hooks: [stop] }] },

    maxTurns: spec.maxTurns,
    maxBudgetUsd: spec.maxBudgetUsd,
    abortController: spec.abortController,

    env: {
      PATH: '/usr/local/bin:/usr/bin:/bin',
      CLAUDE_CONFIG_DIR: spec.configDir,
      CLAUDE_CODE_DISABLE_AUTO_MEMORY: '1',
      CLAUDE_CODE_DISABLE_CLAUDE_MDS: '1',
      CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: '1',
      ...spec.env,
    },
    spawnClaudeCodeProcess: spec.spawn,
  };
}
