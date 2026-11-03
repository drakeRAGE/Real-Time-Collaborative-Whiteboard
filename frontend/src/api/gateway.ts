import { GetHistoryResponse as BoardHistory } from '../realtime/gen/whiteboard/board/v1/board';
import { GetHistoryResponse as ChatHistory } from '../realtime/gen/whiteboard/chat/v1/chat';
import { CreateRoomResponse } from '../realtime/gen/whiteboard/room/v1/room';
import { drawOpsFromHistory, type DrawOp } from '../utils/canvasUtils';

function backend(): string {
  return (import.meta.env.VITE_BACKEND_URL || 'http://localhost:8080').replace(/\/$/, '');
}

function authHeaders(token: string): HeadersInit {
  return { Authorization: `Bearer ${token}` };
}

async function errorMessage(res: Response): Promise<string> {
  try {
    const body: unknown = await res.json();
    if (body && typeof body === 'object' && 'message' in body && typeof body.message === 'string') {
      return body.message;
    }
  } catch {
    return res.statusText || 'Request failed';
  }
  return res.statusText || 'Request failed';
}

export async function createRoom(token: string, title: string): Promise<string> {
  const res = await fetch(`${backend()}/api/v1/rooms`, {
    method: 'POST',
    headers: { ...authHeaders(token), 'Content-Type': 'application/json' },
    body: JSON.stringify({ title }),
  });
  if (!res.ok) throw new Error(await errorMessage(res));
  const parsed = CreateRoomResponse.fromJSON(await res.json());
  const id = parsed.room?.id;
  if (!id) throw new Error('Room was not created');
  return id;
}

export async function deleteRoom(token: string, roomId: string): Promise<void> {
  const res = await fetch(`${backend()}/api/v1/rooms/${roomId}`, {
    method: 'DELETE',
    headers: authHeaders(token),
  });
  if (!res.ok) throw new Error(await errorMessage(res));
}

/**
 * Gateway REST history is not mounted yet (board/chat history stay on gRPC).
 * A non-OK response means "start empty"; live events still draw.
 */
export async function loadBoardHistory(roomId: string, token: string): Promise<DrawOp[]> {
  try {
    const res = await fetch(`${backend()}/api/v1/rooms/${roomId}/board/history`, {
      headers: authHeaders(token),
    });
    if (!res.ok) return [];
    const parsed = BoardHistory.fromJSON(await res.json());
    return drawOpsFromHistory(parsed.ops);
  } catch {
    return [];
  }
}

export interface ChatRecord {
  id: string;
  userId: string;
  username: string;
  body: string;
  createdAt: string;
}

export async function loadChatHistory(roomId: string, token: string): Promise<ChatRecord[]> {
  try {
    const res = await fetch(`${backend()}/api/v1/rooms/${roomId}/chat/history`, {
      headers: authHeaders(token),
    });
    if (!res.ok) return [];
    const parsed = ChatHistory.fromJSON(await res.json());
    return parsed.messages.map((msg) => ({
      id: msg.id,
      userId: msg.userId,
      username: msg.username,
      body: msg.body,
      createdAt: msg.createdAt ? msg.createdAt.toISOString() : new Date().toISOString(),
    }));
  } catch {
    return [];
  }
}
