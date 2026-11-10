import { getProfile, type UserProfile } from './api'
import { getAccessToken } from './token-storage'

// Retrieve the account associated with the saved login token
export async function getCurrentUser(): Promise<UserProfile> {
    const accessToken = await getAccessToken();

    if (!accessToken) {
        throw new Error('Please sign in to continue');
    }

    return getProfile(accessToken);
}