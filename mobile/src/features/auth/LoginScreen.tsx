import { Pressable, StyleSheet, Text, TextInput } from 'react-native';
import { SafeAreaView } from 'react-native-safe-area-context';
import { useState } from 'react';
import { login } from './api';
import { saveAccessToken } from './token-storage'
import { getCurrentUser } from './session'
// Display Login screen
export default function LoginScreen() {
  // Remember what the user types
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [success, setSuccess] = useState('');

  // Verify crednetials through the backend and save its token to device-backed storage
  async function handleSignIn() {
    if (isSubmitting) {
      return;
    }

    setError('');
    setSuccess('');

    if (email.trim() === '' || password === '') {
      setError('Please enter your email and password.');
      return;
    }

    setIsSubmitting(true)

    try {
      const result = await login(email.trim(), password);
      await saveAccessToken(result.access_token);

      const user = await getCurrentUser();

      setPassword('');
      setSuccess(`Signed in as ${user.username}.`)
    } catch (error) {
      setError(
        error instanceof Error ? error.message : 'Unable to sign in',
      );
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <SafeAreaView style={styles.container}>
      <Text style={styles.title}>TeamFinder</Text>
      <Text style={styles.description}>
        Find a teammate. Play together. Talk together.
      </Text>
      <TextInput
        style={styles.input}
        placeholder="Email"
        placeholderTextColor="#6b7280"
        accessibilityLabel="Email"
        value={email}
        onChangeText={setEmail}
        autoCapitalize="none"
        autoCorrect={false}
        keyboardType="email-address"
        autoComplete="email"
        editable={!isSubmitting}
      />

      <TextInput
        style={styles.input}
        placeholder="Password"
        placeholderTextColor="#6b7280"
        accessibilityLabel="Password"
        value={password}
        onChangeText={setPassword}
        autoCapitalize="none"
        autoCorrect={false}
        secureTextEntry
        autoComplete="current-password"
        editable={!isSubmitting}
      />

      {error !== '' && (
        <Text style={styles.error} accessibilityRole="alert">
          {error}
        </Text>
      )}

      {success !== '' && (
        <Text style={styles.description}>{success}</Text>
      )}

      <Pressable
        accessibilityRole="button"
        disabled={isSubmitting}
        onPress={handleSignIn}
        style={({ pressed }) => [
          styles.button,
          pressed && styles.buttonPressed,
        ]}
      >
        <Text style={styles.buttonText}>
          {isSubmitting ? 'Signing in...' : 'Sign in'}
        </Text>
      </Pressable>
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    justifyContent: 'center',
    alignItems: 'center',
    padding: 24,
    backgroundColor: '#ffffff',
  },
  title: {
    fontSize: 32,
    fontWeight: 'bold',
    color: '#111827',
  },
  description: {
    marginTop: 12,
    fontSize: 16,
    textAlign: 'center',
    color: '#4b5563',
  },
  input: {
    width: '100%',
    maxWidth: 360,
    marginTop: 16,
    padding: 14,
    borderWidth: 1,
    borderColor: '#d1d5db',
    borderRadius: 8,
    fontSize: 16,
    color: '#111827'
  },
  error: {
    marginTop: 12,
    color: '#b91c1c',
    textAlign: 'center',
  },
  button: {
    width: '100%',
    maxWidth: 360,
    marginTop: 20,
    padding: 14,
    borderRadius: 8,
    backgroundColor: '#2563eb',
    alignItems: 'center'
  },
  buttonPressed: {
    opacity: 0.7,
  },
  buttonText: {
    color: '#ffffff',
    fontSize: 16,
    fontWeight: '600'
  }
});
