import { createContext, useContext, useState, useCallback, type ReactNode } from "react";

export interface Notification {
  id: string;
  title: string;
  description?: string;
  type: "info" | "warning" | "error";
  action?: { label: string; onClick: () => void };
  persistent?: boolean; // won't auto-dismiss
  onDismiss?: () => void; // called when user clicks X to dismiss
}

interface NotificationContextType {
  notifications: Notification[];
  addNotification: (n: Omit<Notification, "id">) => string;
  removeNotification: (id: string) => void;
  clearAll: () => void;
}

const NotificationContext = createContext<NotificationContextType | null>(null);

export function NotificationProvider({ children }: { children: ReactNode }) {
  const [notifications, setNotifications] = useState<Notification[]>([]);

  const addNotification = useCallback((n: Omit<Notification, "id">) => {
    const id = Date.now().toString(36) + Math.random().toString(36).slice(2, 6);
    setNotifications(prev => {
      // Don't duplicate by title
      if (prev.some(p => p.title === n.title)) return prev;
      return [...prev, { ...n, id }];
    });
    return id;
  }, []);

  const removeNotification = useCallback((id: string) => {
    setNotifications(prev => prev.filter(n => n.id !== id));
  }, []);

  const clearAll = useCallback(() => {
    setNotifications([]);
  }, []);

  return (
    <NotificationContext.Provider value={{ notifications, addNotification, removeNotification, clearAll }}>
      {children}
    </NotificationContext.Provider>
  );
}

export function useNotifications() {
  const ctx = useContext(NotificationContext);
  if (!ctx) throw new Error("useNotifications must be used within NotificationProvider");
  return ctx;
}
