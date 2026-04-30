import { useState, useEffect, useCallback, useMemo, FormEvent } from 'react';
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
import { AgentClient } from './agent-client';

export function App() {
  const client = useMemo(() => new AgentClient(), []);

  const processor = useMemo(() => {
    return new MessageProcessor<ReactComponentImplementation>([basicCatalog], (action) => {
      console.log('User action:', action);
    });
  }, []);

  const [sessionId, setSessionId] = useState<string>('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [input, setInput] = useState('创建一个登录表单，包含用户名、密码输入框和提交按钮');
  const [agentText, setAgentText] = useState('');
  const [toolCalls, setToolCalls] = useState<string[]>([]);
  const [surfaces, setSurfaces] = useState<
    SurfaceModel<ReactComponentImplementation>[]
  >([]);

  useEffect(() => {
    const sub1 = processor.onSurfaceCreated((surface) => {
      setSurfaces((prev) => [...prev, surface]);
    });
    const sub2 = processor.onSurfaceDeleted((id) => {
      setSurfaces((prev) => prev.filter((s) => s.id !== id));
    });
    return () => {
      sub1.unsubscribe();
      sub2.unsubscribe();
    };
  }, [processor]);

  const handleSubmit = useCallback(
    async (e: FormEvent) => {
      e.preventDefault();
      if (!input.trim() || loading) return;

      setLoading(true);
      setError(null);
      setAgentText('');
      setToolCalls([]);
      setSurfaces([]);

      // Clear existing surfaces
      Array.from(processor.model.surfacesMap.keys()).forEach((id) => {
        processor.model.deleteSurface(id);
      });

      try {
        const response = await client.chat(input, sessionId || undefined);
        setSessionId(response.session_id);
        setAgentText(response.text);

        // Process A2UI messages — deduplicate and handle errors gracefully
        const a2uiMsgs = client.extractA2UIMessages(response);
        if (a2uiMsgs.length > 0) {
          // Deduplicate: keep only the last createSurface for each surfaceId,
          // and the last updateComponents for each surfaceId
          const seen = new Map<string, A2uiClientMessage>();
          for (const msg of a2uiMsgs) {
            const kind = Object.keys(msg).find((k) => k !== 'version') || '';
            const payload = (msg as Record<string, unknown>)[kind] as Record<string, unknown> | undefined;
            const surfaceId = payload?.surfaceId as string || 'default';
            const key = `${kind}:${surfaceId}`;
            seen.set(key, msg);
          }
          const deduped = Array.from(seen.values());

          try {
            processor.processMessages(deduped);
          } catch (err) {
            console.warn('processMessages error (non-fatal):', err);
          }
          setToolCalls(deduped.map((m) => Object.keys(m).find((k) => k !== 'version') || 'unknown'));
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        setLoading(false);
      }
    },
    [input, loading, sessionId, client, processor]
  );

  const handleNewSession = useCallback(() => {
    setSessionId('');
    setAgentText('');
    setToolCalls([]);
    setError(null);
    Array.from(processor.model.surfacesMap.keys()).forEach((id) => {
      processor.model.deleteSurface(id);
    });
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
          {/* Left panel: controls */}
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

          {/* Right panel: A2UI rendered output */}
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
