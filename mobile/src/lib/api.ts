// Share the backed address across API modules
export const API_BASE_URL = process.env.EXPO_PUBLIC_API_URL;

if (!API_BASE_URL) {
    throw new Error('EXPO_PUBLIC_API_URL is not configured');
}