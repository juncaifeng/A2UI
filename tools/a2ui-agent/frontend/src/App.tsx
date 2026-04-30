import { useState, useEffect, useCallback, useMemo, useRef, FormEvent } from 'react';
import {
  A2uiSurface,
  basicCatalog,
  MarkdownContext,
} from '@a2ui/react/v0_9';
import {
  A2uiClientMessage,
  MessageProcessor,
  SurfaceModel,
  ReactComponentImplementation,
} from '@a2ui/web_core/v0_9';
import { renderMarkdown } from '@a2ui/markdown-it';
import { AgentClient, ChatResponse } from './agent-client';

type ActionPayload = {
  name: string;
  surfaceId: string;
  sourceComponentId: string;
  context: Record<string, unknown>;
};

function processA2UI(
  response: ChatResponse,
  client: AgentClient,
  processor: MessageProcessor<ReactComponentImplementation>,
): { msgs: A2uiClientMessage[]; kinds: string[] } {
  const a2uiMsgs = client.extractA2UIMessages(response);
  if (a2uiMsgs.length === 0) return { msgs: [], kinds: [] };
  const seen = new Map<string, A2uiClientMessage>();
  for (const msg of a2uiMsgs) {
    const kind = Object.keys(msg).find((k) => k !== 'version') || '';
    const payload = (msg as Record<string, unknown>)[kind] as Record<string, unknown> | undefined;
    const surfaceId = (payload?.surfaceId as string) || 'default';
    seen.set(`${kind}:${surfaceId}`, msg);
  }
  const deduped = Array.from(seen.values());
  try { processor.processMessages(deduped); } catch (err) { console.warn('processMessages error:', err); }
  return { msgs: deduped, kinds: deduped.map((m) => Object.keys(m).find((k) => k !== 'version') || 'unknown') };
}

export function App() {
  const client = useMemo(() => new AgentClient(), []);

  const [sessionId, setSessionId] = useState<string>('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [input, setInput] = useState('创建一个登录表单，包含用户名、密码输入框和提交按钮');
  const [agentText, setAgentText] = useState('');
  const [toolCalls, setToolCalls] = useState<string[]>([]);
  const [surfaces, setSurfaces] = useState<SurfaceModel<ReactComponentImplementation>[]>([]);

  // Refs to avoid circular deps between processor and handleAction
  const sessionIdRef = useRef(sessionId);
  sessionIdRef.current = sessionId;
  const actionHandlerRef = useRef<(action: ActionPayload) => void>(() => {});

  const processor = useMemo(() => {
    return new MessageProcessor<ReactComponentImplementation>([basicCatalog], (action: ActionPayload) => {
      actionHandlerRef.current(action);
    });
  }, []);

  useEffect(() => {
    const sub1 = processor.onSurfaceCreated((surface: SurfaceModel<ReactComponentImplementation>) => setSurfaces((prev) => [...prev, surface]));
    const sub2 = processor.onSurfaceDeleted((id: string) => setSurfaces((prev) => prev.filter((s) => s.id !== id)));
    return () => { sub1.unsubscribe(); sub2.unsubscribe(); };
  }, [processor]);

  // Action handler: sends user action back to agent
  useEffect(() => {
    actionHandlerRef.current = async (action: ActionPayload) => {
      console.log('User action:', action);
      const actionMsg = `[用户操作] event: ${action.name}, component: ${action.sourceComponentId}, data: ${JSON.stringify(action.context)}`;
      setInput(actionMsg);
      setLoading(true);
      setError(null);
      try {
        const response = await client.chat(actionMsg, sessionIdRef.current || undefined);
        setSessionId(response.session_id);
        setAgentText(response.text);
        const { kinds } = processA2UI(response, client, processor);
        setToolCalls(kinds);
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        setLoading(false);
      }
    };
  }, [client, processor]);

  const handleSubmit = useCallback(
    async (e: FormEvent) => {
      e.preventDefault();
      if (!input.trim() || loading) return;

      setLoading(true);
      setError(null);
      setAgentText('');
      setToolCalls([]);
      setSurfaces([]);

      Array.from(processor.model.surfacesMap.keys()).forEach((id) => processor.model.deleteSurface(id));

      try {
        const response = await client.chat(input, sessionIdRef.current || undefined);
        setSessionId(response.session_id);
        setAgentText(response.text);
        const { kinds } = processA2UI(response, client, processor);
        setToolCalls(kinds);
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        setLoading(false);
      }
    },
    [input, loading, client, processor],
  );

  const handleNewSession = useCallback(() => {
    setSessionId('');
    setAgentText('');
    setToolCalls([]);
    setError(null);
    Array.from(processor.model.surfacesMap.keys()).forEach((id) => processor.model.deleteSurface(id));
    setSurfaces([]);
  }, [processor]);

  return (
    <MarkdownContext.Provider value={renderMarkdown}>
      <div className="app">
        <header className="header">
          <h1>A2UI Agent Test</h1>
          <div className="header-info">
            {sessionId && (
              <span className="session-badge">
                Session: {sessionId.slice(0, 8)}...
              </span>
            )}
            <button className="btn-new" onClick={handleNewSession}>
              New Session
            </button>
          </div>
        </header>

        <div className="layout">
          <aside className="panel">
            <form className="chat-form" onSubmit={handleSubmit}>
              <textarea
                value={input}
                onChange={(e) => setInput(e.target.value)}
                placeholder="描述你想要的 UI..."
                rows={4}
                disabled={loading}
              />
              <button type="submit" className="btn-send" disabled={loading}>
                {loading ? 'Generating...' : 'Send'}
              </button>
            </form>

            {error && <div className="error-box">{error}</div>}

            {agentText && (
              <div className="agent-reply">
                <h3>Agent Reply</h3>
                <p>{agentText}</p>
              </div>
            )}

            {toolCalls.length > 0 && (
              <div className="tool-log">
                <h3>A2UI Messages</h3>
                <ul>
                  {toolCalls.map((t, i) => (
                    <li key={i}>{t}</li>
                  ))}
                </ul>
              </div>
            )}
          </aside>

          <main className="render-area">
            {loading && (
              <div className="loading">
                <div className="spinner" />
                <p>Agent is building UI...</p>
              </div>
            )}
            {surfaces.length > 0 && (
              <section className="surfaces">
                {surfaces.map((surface) => (
                  <A2uiSurface key={surface.id} surface={surface} />
                ))}
              </section>
            )}
            {!loading && surfaces.length === 0 && !error && (
              <div className="placeholder">
                <span className="material-symbols-outlined">web</span>
                <p>Send a message to generate A2UI</p>
              </div>
            )}
          </main>
        </div>
      </div>
    </MarkdownContext.Provider>
  );
}
