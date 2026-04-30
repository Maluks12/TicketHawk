import React, { useState, useRef, useCallback } from 'react';
import {
  View,
  Text,
  StyleSheet,
  TextInput,
  TouchableOpacity,
  ScrollView,
  KeyboardAvoidingView,
  Platform,
  ActivityIndicator,
  Animated,
  Pressable,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { Feather, Ionicons } from '@expo/vector-icons';
import { useRouter } from 'expo-router';
import { useAuthStore } from '../../store/authStore';

type Mode = 'login' | 'register';

// ─── Input Field ──────────────────────────────────────────────────────────────

interface FieldProps {
  placeholder: string;
  value: string;
  onChange: (v: string) => void;
  secure?: boolean;
  keyboardType?: 'default' | 'email-address' | 'phone-pad' | 'numeric';
  autoCapitalize?: 'none' | 'words' | 'sentences';
  error?: string;
  returnKeyType?: 'next' | 'done';
  onSubmit?: () => void;
  inputRef?: React.RefObject<TextInput | null>;
}

const Field = ({
  placeholder, value, onChange, secure, keyboardType = 'default',
  autoCapitalize = 'none', error, returnKeyType = 'next', onSubmit, inputRef,
}: FieldProps) => {
  const [focused, setFocused] = useState(false);
  const [shown, setShown] = useState(false);

  return (
    <View style={fStyles.wrap}>
      <View style={[
        fStyles.box,
        focused && fStyles.boxFocused,
        !!error  && fStyles.boxError,
      ]}>
        <TextInput
          ref={inputRef}
          style={fStyles.input}
          value={value}
          onChangeText={onChange}
          placeholder={placeholder}
          placeholderTextColor="rgba(10,10,10,0.32)"
          secureTextEntry={secure && !shown}
          keyboardType={keyboardType}
          autoCapitalize={autoCapitalize}
          autoCorrect={false}
          returnKeyType={returnKeyType}
          onFocus={() => setFocused(true)}
          onBlur={() => setFocused(false)}
          onSubmitEditing={onSubmit}
        />
        {secure && (
          <TouchableOpacity onPress={() => setShown(s => !s)} hitSlop={{ top: 8, bottom: 8, left: 8, right: 8 }}>
            <Feather name={shown ? 'eye-off' : 'eye'} size={19} color="rgba(10,10,10,0.4)" />
          </TouchableOpacity>
        )}
      </View>
      {error ? <Text style={fStyles.err}>{error}</Text> : null}
    </View>
  );
};

const fStyles = StyleSheet.create({
  wrap: { gap: 5 },
  box: {
    flexDirection: 'row',
    alignItems: 'center',
    backgroundColor: '#fff',
    borderRadius: 14,
    borderWidth: 1.5,
    borderColor: '#e8e8e8',
    paddingHorizontal: 18,
    paddingVertical: Platform.OS === 'ios' ? 16 : 4,
  },
  boxFocused: { borderColor: '#0a0a0a' },
  boxError:   { borderColor: '#ff3c6e' },
  input: { flex: 1, fontSize: 15.5, color: '#0a0a0a', fontWeight: '400' },
  err: { fontSize: 12, color: '#ff3c6e', fontWeight: '500', marginLeft: 4 },
});

// ─── Checkbox ─────────────────────────────────────────────────────────────────

const Checkbox = ({ checked, onToggle }: { checked: boolean; onToggle: () => void }) => (
  <TouchableOpacity onPress={onToggle} activeOpacity={0.7} style={cbStyles.box}>
    {checked && <Ionicons name="checkmark" size={13} color="#fff" />}
  </TouchableOpacity>
);

const cbStyles = StyleSheet.create({
  box: {
    width: 20,
    height: 20,
    borderRadius: 5,
    borderWidth: 1.5,
    borderColor: '#ccc',
    backgroundColor: 'transparent',
    alignItems: 'center',
    justifyContent: 'center',
  },
});

// ─── Screen ───────────────────────────────────────────────────────────────────

export default function AuthScreen() {
  const insets = useSafeAreaInsets();
  const router  = useRouter();
  const { login, register } = useAuthStore();

  const [mode, setMode]         = useState<Mode>('login');
  const [loading, setLoading]   = useState(false);
  const [globalErr, setGlobalErr] = useState<string | null>(null);
  const [agreed, setAgreed]     = useState(false);
  const tabAnim = useRef(new Animated.Value(0)).current;

  // Login fields
  const [loginEmail, setLoginEmail]       = useState('');
  const [loginPassword, setLoginPassword] = useState('');

  // Register fields
  const [firstName,   setFirstName]   = useState('');
  const [lastName,    setLastName]    = useState('');
  const [regEmail,    setRegEmail]    = useState('');
  const [phone,       setPhone]       = useState('');
  const [dob,         setDob]         = useState('');
  const [regPassword, setRegPassword] = useState('');
  const [confirmPass, setConfirmPass] = useState('');

  const [errors, setErrors] = useState<Record<string, string>>({});

  // Refs for chaining
  const loginPassRef = useRef<TextInput>(null);
  const lastNameRef  = useRef<TextInput>(null);
  const emailRef     = useRef<TextInput>(null);
  const phoneRef     = useRef<TextInput>(null);
  const dobRef       = useRef<TextInput>(null);
  const passRef      = useRef<TextInput>(null);
  const confirmRef   = useRef<TextInput>(null);

  const clearErr = (key: string) =>
    setErrors(prev => ({ ...prev, [key]: '' }));

  const switchMode = (m: Mode) => {
    setMode(m);
    setGlobalErr(null);
    setErrors({});
    Animated.spring(tabAnim, {
      toValue: m === 'login' ? 0 : 1,
      useNativeDriver: false,
      tension: 140,
      friction: 11,
    }).start();
  };

  // Auto-format DOB as DD/MM/YYYY
  const formatDob = (raw: string) => {
    const d = raw.replace(/\D/g, '').slice(0, 8);
    if (d.length > 4) return `${d.slice(0,2)}/${d.slice(2,4)}/${d.slice(4)}`;
    if (d.length > 2) return `${d.slice(0,2)}/${d.slice(2)}`;
    return d;
  };

  const parseDobToISO = (formatted: string) => {
    const [dd, mm, yyyy] = formatted.split('/');
    if (!dd || !mm || !yyyy || yyyy.length < 4) return null;
    const d = new Date(`${yyyy}-${mm}-${dd}`);
    return isNaN(d.getTime()) ? null : d.toISOString();
  };

  // ── Login submit
  const handleLogin = useCallback(async () => {
    const errs: Record<string, string> = {};
    if (!loginEmail.trim())    errs.loginEmail    = 'Please enter your email';
    if (!loginPassword.trim()) errs.loginPassword = 'Please enter your password';
    if (Object.keys(errs).length) { setErrors(errs); return; }

    setLoading(true);
    setGlobalErr(null);
    try {
      await login(loginEmail.trim().toLowerCase(), loginPassword);
      router.back();
    } catch (e: any) {
      const msg = e?.response?.data?.error ?? '';
      setGlobalErr(
        msg.includes('credentials') ? 'Incorrect email or password.' :
        msg || 'Something went wrong. Please try again.'
      );
    } finally { setLoading(false); }
  }, [loginEmail, loginPassword, login, router]);

  // ── Register submit
  const handleRegister = useCallback(async () => {
    const errs: Record<string, string> = {};
    if (!firstName.trim()) errs.firstName = 'Required';
    if (!lastName.trim())  errs.lastName  = 'Required';
    if (!regEmail.trim())  errs.regEmail  = 'Required';
    else if (!/\S+@\S+\.\S+/.test(regEmail)) errs.regEmail = 'Enter a valid email';
    if (!phone.trim())     errs.phone = 'Required';
    else if (phone.replace(/\D/g,'').length < 10) errs.phone = 'Enter a valid number';
    if (!dob)              errs.dob = 'Required';
    else if (!parseDobToISO(dob)) errs.dob = 'Use DD/MM/YYYY';
    if (!regPassword)      errs.regPassword = 'Required';
    else if (regPassword.length < 8) errs.regPassword = 'Min. 8 characters';
    if (confirmPass !== regPassword) errs.confirmPass = 'Passwords don\'t match';
    if (!agreed)           errs.agreed = 'You must agree to continue';
    if (Object.keys(errs).length) { setErrors(errs); return; }

    setLoading(true);
    setGlobalErr(null);
    try {
      await register({
        email:         regEmail.trim().toLowerCase(),
        password:      regPassword,
        first_name:    firstName.trim(),
        last_name:     lastName.trim(),
        phone_number:  phone.trim(),
        date_of_birth: parseDobToISO(dob)!,
      });
      router.back();
    } catch (e: any) {
      const msg = e?.response?.data?.error ?? '';
      setGlobalErr(
        msg.toLowerCase().includes('email') ? 'An account with this email already exists.' :
        msg || 'Something went wrong. Please try again.'
      );
    } finally { setLoading(false); }
  }, [firstName, lastName, regEmail, phone, dob, regPassword, confirmPass, agreed, register, router]);

  const indicatorLeft = tabAnim.interpolate({
    inputRange: [0, 1],
    outputRange: ['2%', '51%'],
  });

  return (
    <View style={styles.root}>
      <KeyboardAvoidingView
        style={{ flex: 1 }}
        behavior={Platform.OS === 'ios' ? 'padding' : 'height'}
      >
        <ScrollView
          contentContainerStyle={[styles.scroll, { paddingBottom: insets.bottom + 48 }]}
          keyboardShouldPersistTaps="handled"
          showsVerticalScrollIndicator={false}
        >
          {/* Close button */}
          <TouchableOpacity
            style={[styles.closeBtn, { marginTop: insets.top + 14 }]}
            onPress={() => router.back()}
            hitSlop={{ top: 8, bottom: 8, left: 8, right: 8 }}
          >
            <Feather name="x" size={20} color="#0a0a0a" />
          </TouchableOpacity>

          {/* Branding */}
          <View style={styles.brand}>
            <View style={styles.logoMark}>
              <Ionicons name="ticket" size={26} color="#fff" />
            </View>
            <Text style={styles.brandTitle}>
              {mode === 'login' ? 'Welcome back' : 'Create account'}
            </Text>
            <Text style={styles.brandSub}>
              {mode === 'login'
                ? 'Sign in to access your tickets and bookings'
                : 'Join thousands of event-goers across South Africa'}
            </Text>
          </View>

          {/* Tab pill */}
          <View style={styles.tabTrack}>
            <Animated.View style={[styles.tabThumb, { left: indicatorLeft }]} />
            <TouchableOpacity style={styles.tab} onPress={() => switchMode('login')} activeOpacity={0.75}>
              <Text style={[styles.tabLabel, mode === 'login' && styles.tabLabelActive]}>Log in</Text>
            </TouchableOpacity>
            <TouchableOpacity style={styles.tab} onPress={() => switchMode('register')} activeOpacity={0.75}>
              <Text style={[styles.tabLabel, mode === 'register' && styles.tabLabelActive]}>Sign up</Text>
            </TouchableOpacity>
          </View>

          {/* Global error */}
          {globalErr && (
            <View style={styles.errBanner}>
              <Ionicons name="alert-circle-outline" size={15} color="#ff3c6e" />
              <Text style={styles.errBannerText}>{globalErr}</Text>
            </View>
          )}

          {/* ─── LOGIN FORM ─── */}
          {mode === 'login' && (
            <View style={styles.form}>
              <Field
                placeholder="Email address"
                value={loginEmail}
                onChange={v => { setLoginEmail(v); clearErr('loginEmail'); }}
                keyboardType="email-address"
                error={errors.loginEmail}
                returnKeyType="next"
                onSubmit={() => loginPassRef.current?.focus()}
              />
              <Field
                placeholder="Password"
                value={loginPassword}
                onChange={v => { setLoginPassword(v); clearErr('loginPassword'); }}
                secure
                inputRef={loginPassRef}
                error={errors.loginPassword}
                returnKeyType="done"
                onSubmit={handleLogin}
              />

              <TouchableOpacity onPress={() => {}} style={styles.forgotWrap}>
                <Text style={styles.forgotText}>Forgot password?</Text>
              </TouchableOpacity>

              <TouchableOpacity
                style={[styles.cta, loading && styles.ctaDisabled]}
                onPress={handleLogin}
                activeOpacity={0.85}
                disabled={loading}
              >
                {loading
                  ? <ActivityIndicator color="#fff" size="small" />
                  : <Text style={styles.ctaText}>Log in</Text>
                }
              </TouchableOpacity>

              <TouchableOpacity onPress={() => switchMode('register')} style={styles.switchRow}>
                <Text style={styles.switchText}>
                  Don't have an account?{'  '}
                  <Text style={styles.switchLink}>Sign up</Text>
                </Text>
              </TouchableOpacity>
            </View>
          )}

          {/* ─── REGISTER FORM ─── */}
          {mode === 'register' && (
            <View style={styles.form}>
              {/* Email */}
              <Field
                placeholder="Email address"
                value={regEmail}
                onChange={v => { setRegEmail(v); clearErr('regEmail'); }}
                keyboardType="email-address"
                error={errors.regEmail}
                returnKeyType="next"
                onSubmit={() => lastNameRef.current?.focus()}
              />

              {/* Name row */}
              <View style={styles.nameRow}>
                <View style={{ flex: 1 }}>
                  <Field
                    placeholder="First name"
                    value={firstName}
                    onChange={v => { setFirstName(v); clearErr('firstName'); }}
                    autoCapitalize="words"
                    error={errors.firstName}
                    returnKeyType="next"
                    onSubmit={() => lastNameRef.current?.focus()}
                  />
                </View>
                <View style={{ flex: 1 }}>
                  <Field
                    placeholder="Last name"
                    value={lastName}
                    onChange={v => { setLastName(v); clearErr('lastName'); }}
                    autoCapitalize="words"
                    inputRef={lastNameRef}
                    error={errors.lastName}
                    returnKeyType="next"
                    onSubmit={() => phoneRef.current?.focus()}
                  />
                </View>
              </View>

              {/* Password */}
              <Field
                placeholder="Password"
                value={regPassword}
                onChange={v => { setRegPassword(v); clearErr('regPassword'); }}
                secure
                inputRef={passRef}
                error={errors.regPassword}
                returnKeyType="next"
                onSubmit={() => phoneRef.current?.focus()}
              />

              {/* Phone */}
              <View style={styles.phoneRow}>
                <View style={styles.dialCode}>
                  <Text style={styles.dialFlag}>🇿🇦</Text>
                  <Text style={styles.dialNum}>+27</Text>
                </View>
                <View style={{ flex: 1 }}>
                  <Field
                    placeholder="Phone number"
                    value={phone}
                    onChange={v => { setPhone(v); clearErr('phone'); }}
                    keyboardType="phone-pad"
                    inputRef={phoneRef}
                    error={errors.phone}
                    returnKeyType="next"
                    onSubmit={() => dobRef.current?.focus()}
                  />
                </View>
              </View>

              {/* Date of birth */}
              <Field
                placeholder="Date of birth (DD/MM/YYYY)"
                value={dob}
                onChange={v => { setDob(formatDob(v)); clearErr('dob'); }}
                keyboardType="numeric"
                inputRef={dobRef}
                error={errors.dob}
                returnKeyType="next"
                onSubmit={() => confirmRef.current?.focus()}
              />

              {/* Confirm password */}
              <Field
                placeholder="Confirm password"
                value={confirmPass}
                onChange={v => { setConfirmPass(v); clearErr('confirmPass'); }}
                secure
                inputRef={confirmRef}
                error={errors.confirmPass}
                returnKeyType="done"
                onSubmit={handleRegister}
              />

              {/* Terms checkbox */}
              <TouchableOpacity
                style={styles.termsRow}
                onPress={() => { setAgreed(a => !a); clearErr('agreed'); }}
                activeOpacity={0.75}
              >
                <View style={[styles.checkbox, agreed && styles.checkboxChecked]}>
                  {agreed && <Ionicons name="checkmark" size={13} color="#fff" strokeWidth={3} />}
                </View>
                <Text style={styles.termsText}>
                  By checking this box, you agree to our{' '}
                  <Text style={styles.termsLink}>Terms of Use</Text>
                  {' '}and acknowledge having read our{' '}
                  <Text style={styles.termsLink}>Privacy Policy</Text>.
                </Text>
              </TouchableOpacity>
              {errors.agreed ? <Text style={styles.termsErr}>{errors.agreed}</Text> : null}

              {/* CTA */}
              <TouchableOpacity
                style={[styles.cta, loading && styles.ctaDisabled]}
                onPress={handleRegister}
                activeOpacity={0.85}
                disabled={loading}
              >
                {loading
                  ? <ActivityIndicator color="#fff" size="small" />
                  : <Text style={styles.ctaText}>Sign up</Text>
                }
              </TouchableOpacity>

              <TouchableOpacity onPress={() => switchMode('login')} style={styles.switchRow}>
                <Text style={styles.switchText}>
                  Already have an account?{'  '}
                  <Text style={styles.switchLink}>Log in</Text>
                </Text>
              </TouchableOpacity>
            </View>
          )}
        </ScrollView>
      </KeyboardAvoidingView>
    </View>
  );
}

// ─── Styles ───────────────────────────────────────────────────────────────────

const styles = StyleSheet.create({
  root: { flex: 1, backgroundColor: '#fafafa' },

  scroll: { paddingHorizontal: 22 },

  // Close
  closeBtn: {
    width: 36,
    height: 36,
    borderRadius: 10,
    backgroundColor: '#efefef',
    alignItems: 'center',
    justifyContent: 'center',
    alignSelf: 'flex-start',
  },

  // Brand
  brand: { alignItems: 'center', marginTop: 28, marginBottom: 30, gap: 10 },
  logoMark: {
    width: 56,
    height: 56,
    borderRadius: 16,
    backgroundColor: '#ff3c6e',
    alignItems: 'center',
    justifyContent: 'center',
    marginBottom: 2,
    shadowColor: '#ff3c6e',
    shadowOffset: { width: 0, height: 6 },
    shadowOpacity: 0.35,
    shadowRadius: 14,
    elevation: 8,
  },
  brandTitle: { fontSize: 24, fontWeight: '800', color: '#0a0a0a', letterSpacing: -0.5 },
  brandSub:   { fontSize: 14, color: 'rgba(10,10,10,0.42)', textAlign: 'center', lineHeight: 20, maxWidth: 270 },

  // Tabs
  tabTrack: {
    flexDirection: 'row',
    backgroundColor: '#ebebeb',
    borderRadius: 14,
    padding: 4,
    marginBottom: 24,
    position: 'relative',
  },
  tabThumb: {
    position: 'absolute',
    top: 4,
    bottom: 4,
    width: '48%',
    backgroundColor: '#fff',
    borderRadius: 11,
    shadowColor: '#000',
    shadowOffset: { width: 0, height: 1 },
    shadowOpacity: 0.1,
    shadowRadius: 4,
    elevation: 2,
  },
  tab:           { flex: 1, alignItems: 'center', paddingVertical: 11, zIndex: 1 },
  tabLabel:      { fontSize: 14, fontWeight: '600', color: 'rgba(10,10,10,0.38)' },
  tabLabelActive:{ color: '#0a0a0a', fontWeight: '700' },

  // Error banner
  errBanner: {
    flexDirection: 'row',
    alignItems: 'flex-start',
    gap: 8,
    backgroundColor: 'rgba(255,60,110,0.07)',
    borderWidth: 1,
    borderColor: 'rgba(255,60,110,0.18)',
    borderRadius: 12,
    padding: 13,
    marginBottom: 16,
  },
  errBannerText: { flex: 1, fontSize: 13, color: '#ff3c6e', lineHeight: 18, fontWeight: '500' },

  // Form
  form:    { gap: 14 },
  nameRow: { flexDirection: 'row', gap: 10 },

  // Phone row
  phoneRow: { flexDirection: 'row', alignItems: 'flex-start', gap: 10 },
  dialCode: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
    backgroundColor: '#fff',
    borderRadius: 14,
    borderWidth: 1.5,
    borderColor: '#e8e8e8',
    paddingHorizontal: 14,
    paddingVertical: Platform.OS === 'ios' ? 16 : 14,
  },
  dialFlag: { fontSize: 18 },
  dialNum:  { fontSize: 14, fontWeight: '600', color: '#0a0a0a' },

  // Forgot
  forgotWrap: { alignSelf: 'flex-end', marginTop: -2 },
  forgotText: { fontSize: 13, color: '#0a0a0a', fontWeight: '600', textDecorationLine: 'underline' },

  // Terms
  termsRow: { flexDirection: 'row', alignItems: 'flex-start', gap: 12, marginTop: 2 },
  checkbox: {
    width: 20,
    height: 20,
    borderRadius: 6,
    borderWidth: 1.5,
    borderColor: '#ccc',
    alignItems: 'center',
    justifyContent: 'center',
    marginTop: 1,
    flexShrink: 0,
  },
  checkboxChecked: { backgroundColor: '#0a0a0a', borderColor: '#0a0a0a' },
  termsText: { flex: 1, fontSize: 13, color: 'rgba(10,10,10,0.5)', lineHeight: 19 },
  termsLink: { color: '#0a0a0a', fontWeight: '600' },
  termsErr:  { fontSize: 12, color: '#ff3c6e', fontWeight: '500', marginTop: -6, marginLeft: 32 },

  // CTA
  cta: {
    backgroundColor: '#0a0a0a',
    borderRadius: 14,
    paddingVertical: 17,
    alignItems: 'center',
    marginTop: 4,
    shadowColor: '#000',
    shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.18,
    shadowRadius: 12,
    elevation: 6,
  },
  ctaDisabled: { opacity: 0.55 },
  ctaText:     { color: '#fff', fontSize: 16, fontWeight: '800', letterSpacing: 0.2 },

  // Switch hint
  switchRow: { alignItems: 'center', marginTop: -2, paddingVertical: 4 },
  switchText: { fontSize: 13, color: 'rgba(10,10,10,0.45)' },
  switchLink: { color: '#ff3c6e', fontWeight: '700' },
});