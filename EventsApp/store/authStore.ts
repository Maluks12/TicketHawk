import { create } from 'zustand';
import * as SecureStore from 'expo-secure-store';
import { api } from '../lib/api';
interface AuthState {
  user: { id: string; email: string; role: string; first_name?: string } | null;
  isLoading: boolean;
  login: (email: string, password: string) => Promise<void>;
  register: (dto: RegisterDTO) => Promise<void>;
  logout: () => Promise<void>;
  loadUser: () => Promise<void>;
}

interface RegisterDTO {
  email: string; password: string;
  first_name: string; last_name: string;
  date_of_birth: string; phone_number: string;
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  isLoading: false,

  login: async (email, password) => {
    set({ isLoading: true });
    const { data } = await api.post('/auth/login', { email, password });
    await SecureStore.setItemAsync('access_token',  data.access_token);
    await SecureStore.setItemAsync('refresh_token', data.refresh_token);
    set({ user: data.user, isLoading: false });
  },

  register: async (dto) => {
    set({ isLoading: true });
    const { data } = await api.post('/auth/register', dto);
    await SecureStore.setItemAsync('access_token',  data.access_token);
    await SecureStore.setItemAsync('refresh_token', data.refresh_token);
    set({ user: data.user, isLoading: false });
  },

  logout: async () => {
    await api.post('/auth/logout');
    await SecureStore.deleteItemAsync('access_token');
    await SecureStore.deleteItemAsync('refresh_token');
    set({ user: null });
  },

  loadUser: async () => {
    const token = await SecureStore.getItemAsync('access_token');
    if (!token) return;
    // token exists — decode locally or call a /me endpoint if you add one
    set({ user: { id: '', email: '', role: 'attendee' } });
  },
}));
