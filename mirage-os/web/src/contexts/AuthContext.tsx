// Mirage 认证 Context - 全局认证状态管理
import { createContext, useContext, useState, useEffect, useCallback, ReactNode } from 'react';

export type UserRole = 'admin' | 'operator' | 'viewer' | 'shadow';

export interface AuthState {
  isAuthenticated: boolean;
  userRole: UserRole;
  userId: string | null;
  token: string | null;
  expiresAt: number | null;
}

interface AuthContextType extends AuthState {
  breachProgress: number;
  breachTriggered: boolean;
  accessProgress: number;
  accessTriggered: boolean;
  authenticate: (signature: string, challenge: string) => Promise<boolean>;
  logout: () => void;
  devAuthenticate: () => void;
}

const AuthContext = createContext<AuthContextType | null>(null);

// 硬件指纹采集
const collectHardwareFingerprint = (): string => {
  const canvas = document.createElement('canvas');
  const ctx = canvas.getContext('2d');
  if (ctx) {
    ctx.textBaseline = 'top';
    ctx.font = '14px Arial';
    ctx.fillText('Mirage', 2, 2);
  }
  
  const fingerprint = [
    navigator.userAgent,
    navigator.language,
    screen.width,
    screen.height,
    screen.colorDepth,
    new Date().getTimezoneOffset(),
    canvas.toDataURL(),
    navigator.hardwareConcurrency || 0,
  ].join('|');
  
  let hash = 0;
  for (let i = 0; i < fingerprint.length; i++) {
    const char = fingerprint.charCodeAt(i);
    hash = ((hash << 5) - hash) + char;
    hash = hash & hash;
  }
  return Math.abs(hash).toString(16);
};

// 键盘序列
const BREACH_SEQUENCE = ['m', 'i', 'r', 'a', 'g', 'e'];
const ACCESS_SEQUENCE = ['a', 'c', 'c', 'e', 's', 's'];

export const AuthProvider = ({ children, apiUrl = '/api/auth' }: { children: ReactNode; apiUrl?: string }) => {
  const [authState, setAuthState] = useState<AuthState>({
    isAuthenticated: false,
    userRole: 'shadow',
    userId: null,
    token: null,
    expiresAt: null,
  });
  
  const [breachProgress, setBreachProgress] = useState(0);
  const [breachTriggered, setBreachTriggered] = useState(false);
  const [accessProgress, setAccessProgress] = useState(0);
  const [accessTriggered, setAccessTriggered] = useState(false);

  // 键盘序列监听
  useEffect(() => {
    let sequence: string[] = [];
    let timeout: ReturnType<typeof setTimeout>;

    const handleKeyDown = (e: KeyboardEvent) => {
      clearTimeout(timeout);
      sequence.push(e.key.toLowerCase());
      
      const breachExpected = BREACH_SEQUENCE.slice(0, sequence.length);
      if (sequence.join('') === breachExpected.join('')) {
        setBreachProgress(sequence.length / BREACH_SEQUENCE.length);
        setAccessProgress(0);
        
        if (sequence.length === BREACH_SEQUENCE.length) {
          setBreachTriggered(true);
          setAccessTriggered(false);
          sequence = [];
        }
      } else {
        const accessExpected = ACCESS_SEQUENCE.slice(0, sequence.length);
        if (sequence.join('') === accessExpected.join('')) {
          setAccessProgress(sequence.length / ACCESS_SEQUENCE.length);
          setBreachProgress(0);
          
          if (sequence.length === ACCESS_SEQUENCE.length) {
            setAccessTriggered(true);
            setBreachTriggered(false);
            sequence = [];
          }
        } else {
          sequence = [];
          setBreachProgress(0);
          setAccessProgress(0);
        }
      }
      
      timeout = setTimeout(() => {
        sequence = [];
        setBreachProgress(0);
        setAccessProgress(0);
      }, 3000);
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => {
      window.removeEventListener('keydown', handleKeyDown);
      clearTimeout(timeout);
    };
  }, []);

  // 检查已存储的 Token
  useEffect(() => {
    const token = localStorage.getItem('mirage_token');
    if (token) {
      validateToken(token);
    }
  }, []);

  const validateToken = async (token: string) => {
    try {
      const response = await fetch(`${apiUrl}/validate`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-Mirage-Token': token,
          'X-Hardware-Fingerprint': collectHardwareFingerprint(),
        },
        body: JSON.stringify({ token }),
      });

      if (response.ok) {
        const data = await response.json();
        if (data.success && data.token) {
          setAuthState({
            isAuthenticated: true,
            userRole: data.role || 'viewer',
            userId: data.userId || null,
            token: data.token,
            expiresAt: data.expiresAt || null,
          });
          localStorage.setItem('mirage_token', data.token);
        }
      }
    } catch {
      // Token 验证失败，保持未认证状态
    }
  };

  const authenticate = useCallback(async (signature: string, challenge: string) => {
    try {
      const response = await fetch(`${apiUrl}/breach`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-Hardware-Fingerprint': collectHardwareFingerprint(),
        },
        body: JSON.stringify({
          signature,
          challenge,
          timestamp: Date.now(),
        }),
      });

      if (response.ok) {
        const data = await response.json();
        if (data.success && data.token) {
          setAuthState({
            isAuthenticated: true,
            userRole: data.role || 'viewer',
            userId: data.userId || null,
            token: data.token,
            expiresAt: data.expiresAt || null,
          });
          localStorage.setItem('mirage_token', data.token);
          return true;
        }
      }
      return false;
    } catch {
      return false;
    }
  }, [apiUrl]);

  const logout = useCallback(() => {
    localStorage.removeItem('mirage_token');
    setAuthState({
      isAuthenticated: false,
      userRole: 'shadow',
      userId: null,
      token: null,
      expiresAt: null,
    });
    setBreachTriggered(false);
  }, []);

  const devAuthenticate = useCallback(() => {
    if (typeof import.meta !== 'undefined' && (import.meta as any).env?.DEV) {
      setAuthState({
        isAuthenticated: true,
        userRole: 'admin',
        userId: 'dev-admin',
        token: 'dev-token',
        expiresAt: Date.now() + 86400000,
      });
    }
  }, []);

  return (
    <AuthContext.Provider value={{
      ...authState,
      breachProgress,
      breachTriggered,
      accessProgress,
      accessTriggered,
      authenticate,
      logout,
      devAuthenticate,
    }}>
      {children}
    </AuthContext.Provider>
  );
};

export const useAuthContext = () => {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuthContext must be used within AuthProvider');
  }
  return context;
};
