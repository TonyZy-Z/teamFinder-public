import { createSlice, type PayloadAction } from '@reduxjs/toolkit';
import type { UserProfile } from './api';

type AuthState = {
    status: 'checking' | 'signedOut' | 'signedIn';
    user: UserProfile | null;
};

// Start by checking whether a saved session is still valid
const initialState: AuthState = {
    status: 'checking',
    user: null,
};

const authSlice = createSlice({
    name: 'auth',
    initialState,
    reducers: {
        signedIn(state, action: PayloadAction<UserProfile>) {
            state.status = 'signedIn';
            state.user = action.payload;
        },
        signedOut(state) {
            state.status = 'signedOut';
            state.user = null;
        },
    },
});

export const { signedIn, signedOut } = authSlice.actions;
export default authSlice.reducer;