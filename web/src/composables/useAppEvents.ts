import { onActivated, onDeactivated, onMounted, onUnmounted } from "vue";
import { openEventSource } from "../api/http";

type Handler = (data: string) => void;

let shared: EventSource | null = null;
let opening: Promise<EventSource | null> | null = null;
const handlers = new Set<Handler>();

async function ensureShared(): Promise<EventSource | null> {
  if (shared && shared.readyState !== EventSource.CLOSED) {
    return shared;
  }
  if (!opening) {
    opening = (async () => {
      try {
        const es = await openEventSource("/api/events");
        es.onmessage = (e) => {
          for (const fn of handlers) {
            try {
              fn(e.data);
            } catch {
              /* ignore */
            }
          }
        };
        es.onerror = () => {
          // 让浏览器自行重连；若已彻底关闭则下次 subscribe 重建
          if (es.readyState === EventSource.CLOSED) {
            if (shared === es) shared = null;
          }
        };
        shared = es;
        return es;
      } catch {
        return null;
      } finally {
        opening = null;
      }
    })();
  }
  return opening;
}

function releaseShared() {
  if (handlers.size > 0) return;
  if (shared) {
    shared.close();
    shared = null;
  }
}

/**
 * 全应用共用一条 SSE，避免 keep-alive / 多页面各自建连占满浏览器连接池导致假死。
 * 在 keep-alive 页面也会随 activate/deactivate 增减订阅。
 */
export function useAppEvents(handler: Handler) {
  let active = false;

  async function subscribe() {
    if (active) return;
    active = true;
    handlers.add(handler);
    await ensureShared();
  }

  function unsubscribe() {
    if (!active) return;
    active = false;
    handlers.delete(handler);
    releaseShared();
  }

  onMounted(() => {
    void subscribe();
  });
  onUnmounted(() => {
    unsubscribe();
  });
  onActivated(() => {
    void subscribe();
  });
  onDeactivated(() => {
    unsubscribe();
  });
}
