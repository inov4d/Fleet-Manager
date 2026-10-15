/* WebSocket client with exponential-backoff reconnect; pushes snapshots and events into zustand stores. */
import { wsUrl } from "@/lib/api";
import type { WsMessage } from "@/lib/types";
import { useFleetStore } from "@/stores/fleet";
import { useEventsStore } from "@/stores/events";
import { toast } from "@/stores/ui";

const MIN_BACKOFF_MS = 1000;
const MAX_BACKOFF_MS = 30000;

class FleetSocket {
  private ws: WebSocket | null = null;
  private token: string | null = null;
  private siteId: string | null = null;
  private backoff = MIN_BACKOFF_MS;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private shouldRun = false;
  private attempts = 0;

  connect(token: string, siteId: string) {
    this.token = token;
    this.siteId = siteId;
    this.shouldRun = true;
    this.open();
  }

  /** Change the subscribed site (sends a subscribe message if connected). */
  setSite(siteId: string) {
    this.siteId = siteId;
    this.sendSubscribe();
  }

  disconnect() {
    this.shouldRun = false;
    if (this.reconnectTimer) clearTimeout(this.reconnectTimer);
    this.reconnectTimer = null;
    if (this.ws) {
      const ws = this.ws;
      this.ws = null;
      ws.onclose = null;
      ws.close();
    }
    useFleetStore.getState().setWsStatus("disconnected");
  }

  private open() {
    if (!this.shouldRun || !this.token) return;
    if (this.ws) return;
    useFleetStore.getState().setWsStatus(this.attempts === 0 ? "connecting" : "reconnecting");
    const url = `${wsUrl()}${wsUrl().includes("?") ? "&" : "?"}token=${encodeURIComponent(this.token)}`;
    let ws: WebSocket;
    try {
      ws = new WebSocket(url);
    } catch {
      this.scheduleReconnect();
      return;
    }
    this.ws = ws;

    ws.onopen = () => {
      this.backoff = MIN_BACKOFF_MS;
      this.attempts = 0;
      useFleetStore.getState().setWsStatus("connected");
      this.sendSubscribe();
    };
    ws.onmessage = (ev) => this.handleMessage(ev.data);
    ws.onerror = () => {
      /* onclose follows */
    };
    ws.onclose = () => {
      this.ws = null;
      if (this.shouldRun) this.scheduleReconnect();
    };
  }

  private scheduleReconnect() {
    if (!this.shouldRun || this.reconnectTimer) return;
    this.attempts += 1;
    useFleetStore.getState().setWsStatus("reconnecting");
    const jitter = Math.random() * 300;
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.open();
    }, this.backoff + jitter);
    this.backoff = Math.min(this.backoff * 2, MAX_BACKOFF_MS);
  }

  private sendSubscribe() {
    if (this.ws && this.ws.readyState === WebSocket.OPEN && this.siteId) {
      this.ws.send(JSON.stringify({ type: "subscribe", siteId: this.siteId }));
    }
  }

  private handleMessage(raw: unknown) {
    if (typeof raw !== "string") return;
    let msg: WsMessage;
    try {
      msg = JSON.parse(raw) as WsMessage;
    } catch {
      return;
    }
    if (msg.type === "snapshot") {
      useFleetStore.getState().setSnapshot(msg.ts, msg.data);
    } else if (msg.type === "event") {
      const stored = useEventsStore.getState().push(msg.ts, msg.event);
      switch (stored.kind) {
        case "alarm.raised":
          toast.warning("Alarme levée", stored.message);
          break;
        case "deadlock.detected":
          toast.error("Interblocage détecté", stored.message);
          break;
        case "mission.failed":
          toast.error("Mission échouée", stored.message);
          break;
        case "map.activated":
          useFleetStore.getState().bumpMapReload();
          toast.info("Carte activée", stored.message);
          break;
        default:
          break;
      }
    }
  }
}

export const fleetSocket = new FleetSocket();
