import { executions, type Execution } from '@/lib/mock/store';

// GET /api/automate/v1/executions/{id}/stream — live run status over SSE (E5-S4).
// Emits `data: {execution json}` events and closes once the run reaches a
// terminal state. Mock: replays a short "running → success" progression for a
// live run; for an already-terminal run it emits a single snapshot then closes.
export const dynamic = 'force-dynamic';

const TERMINAL = new Set(['success', 'failed', 'cancelled', 'skipped']);
const encoder = new TextEncoder();

function frame(run: Execution): Uint8Array {
  return encoder.encode(`data: ${JSON.stringify(run)}\n\n`);
}

export async function GET(_req: Request, props: { params: Promise<{ id: string }> }) {
  const params = await props.params;
  const base = executions.find((e) => e.id === params.id);

  const stream = new ReadableStream({
    async start(controller) {
      if (!base) {
        controller.close();
        return;
      }

      // Already-terminal run: emit once and close.
      if (TERMINAL.has(base.status)) {
        controller.enqueue(frame(base));
        controller.close();
        return;
      }

      // Live run: emit the current snapshot, then a couple of progress ticks,
      // then a terminal snapshot, closing the stream (matches the real server).
      const snapshots: Execution[] = [
        base,
        { ...base, status: 'running', durationMs: 4200 },
        { ...base, status: 'success', durationMs: 8800 },
      ];
      for (const snap of snapshots) {
        controller.enqueue(frame(snap));
        await new Promise((r) => setTimeout(r, 400));
      }
      controller.close();
    },
  });

  return new Response(stream, {
    headers: {
      'content-type': 'text/event-stream',
      'cache-control': 'no-cache, no-transform',
      connection: 'keep-alive',
    },
  });
}
