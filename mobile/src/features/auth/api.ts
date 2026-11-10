import { API_BASE_URL } from '@/lib/api'

// Describe the successful response from our Go login endpoint
export type LoginResponse = {
    access_token: string;
    token_type: string;
    expires_in: number;
};

// Describe the account details returned by GET /auth/me
export type UserProfile = {
    id: number;
    email: string;
    username: string;
    created_at: string;
};

// Send credentials to the backend and returns its login response
export async function login(
    email: string,
    password: string,
): Promise<LoginResponse> {
    const response = await fetch(`${API_BASE_URL}/auth/login`, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({email, password}),
    });

    if (!response.ok) {
        const message = await response.text();
        throw new Error(message.trim() || 'Unable to sign in');
    }

    return response.json();
}

// Ask the backend which account belongs to this token
export async function getProfile(
    accessToken: string,
): Promise<UserProfile> {
    const response = await fetch(
        `${API_BASE_URL}/auth/me`, {
        headers: {
            Authorization: `Bearer ${accessToken}`,
        },
    });

    if (!response.ok) {
        const message = await response.text();
        throw new Error(message.trim() || 'Unable to retrieve profile');
    }

    return response.json();
}