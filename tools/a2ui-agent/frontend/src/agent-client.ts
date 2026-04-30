/**
 * Agent client — calls the A2UI Agent REST API via grpc-gateway.
 */
import type { A2uiMessage, A2uiClientMessage } from '@a2ui/web_core/v0_9';

export interface ChatResponse {
  session_id: string;
  sessionId: string;
  text: string;
  a2ui_messages: { kind: string; json: string }[];
  a2uiMessages: { kind: string; json: string }[];
}

export class AgentClient {
  private baseUrl: string;

  constructor(baseUrl = '/v1') {
    this.baseUrl = baseUrl;
  }

  async chat(
    message: string,
    sessionId?: string,
    model?: string
  ): Promise<ChatResponse> {
    const body: Record<string, string> = { message };
    if (sessionId) body.session_id = sessionId;
    if (model) body.model = model;

    const res = await fetch(`${this.baseUrl}/chat`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });

    if (!res.ok) {
      const text = await res.text();
      throw new Error(`Agent error (${res.status}): ${text}`);
    }

    const raw = await res.json();
    // Normalize grpc-gateway camelCase → snake_case
    return {
      session_id: raw.session_id ?? raw.sessionId ?? '',
      sessionId: raw.sessionId ?? raw.session_id ?? '',
      text: raw.text ?? '',
      a2ui_messages: raw.a2ui_messages ?? raw.a2uiMessages ?? [],
      a2uiMessages: raw.a2uiMessages ?? raw.a2ui_messages ?? [],
    };
  }

  /**
   * Extract A2uiMessage[] from agent response.
   */
  extractA2UIMessages(response: ChatResponse): A2uiMessage[] {
    if (!response.a2ui_messages || !Array.isArray(response.a2ui_messages)) {
      return [];
    }
    return response.a2ui_messages.map((m) => JSON.parse(m.json));
  }

  async listModels(): Promise<{ id: string; display_name: string; provider: string }[]> {
    const res = await fetch(`${this.baseUrl}/models`);
    if (!res.ok) throw new Error(`Failed to list models: ${res.status}`);
    const data = await res.json();
    return data.models || [];
  }
}
