import React, { useState, useRef, useCallback } from 'react';
import {
  View, Text, TextInput, TouchableOpacity, Pressable, ScrollView,
  KeyboardAvoidingView, Platform, ActivityIndicator, Animated,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { Feather, Ionicons } from '@expo/vector-icons';
import { CheckCircle } from 'phosphor-react-native';
import { useRouter } from 'expo-router';
import { useAuthStore } from '../../store/authStore';
import { s, sheet, fieldSheet } from '../../styles/auth';
import { useAppTheme } from '../../theme/colors';
import BrandLogo from '../../components/BrandLogo';        // ADD
import { openLegal } from '../../lib/legalLinks';

type Mode = 'login' | 'register';

// ─── DOB helpers (module scope so they never change identity) ────────────────

const formatDob = (raw: string) => {
  const d = raw.replace(/\D/g, '').slice(0, 8);
  if (d.length > 4) return `${d.slice(0,2)}/${d.slice(2,4)}/${d.slice(4)}`;
  if (d.length > 2) return `${d.slice(0,2)}/${d.slice(2)}`;
  return d;
};

const parseDobToISO = (f: string) => {
  const [dd, mm, yyyy] = f.split('/');
  if (!dd || !mm || !yyyy || yyyy.length < 4) return null;
  const d = new Date(`${yyyy}-${mm}-${dd}`);
  return isNaN(d.getTime()) ? null : d.toISOString();
};

// ─── Field ────────────────────────────────────────────────────────────────────

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
  isDark?: boolean;
}

// RESPONSIVENESS: Field is memoised so that typing in one input no longer
// re-renders the other six. Every prop it receives from the screen is a stable
// reference (useCallback / useRef), otherwise memo would never hit.
const Field = React.memo(({ placeholder, value, onChange, secure, keyboardType = 'default',
  autoCapitalize = 'none', error, returnKeyType = 'next', onSubmit, inputRef,
  isDark }: FieldProps) => {
  const [focused, setFocused] = useState(false);
  const [shown, setShown]     = useState(false);

  // Fallback ref for fields the screen doesn't drive, so the box-tap handler
  // below always has something to focus.
  const localRef = useRef<TextInput>(null);
  const ref = inputRef ?? localRef;

  const focusInput = useCallback(() => {
    const node = ref.current;
    if (node && !node.isFocused()) node.focus();
  }, [ref]);

  const handleFocus   = useCallback(() => setFocused(true), []);
  const handleBlur    = useCallback(() => setFocused(false), []);
  const toggleShown   = useCallback(() => setShown(p => !p), []);

  return (
    <View style={fieldSheet.wrap}>
      {/* The box itself is pressable: a tap on the border/padding focuses the
          input instead of being swallowed, which is what made fields feel like
          they needed two taps. */}
      <Pressable
        onPress={focusInput}
        style={[
          fieldSheet.box,
          isDark && { backgroundColor: '#1c1c1c', borderColor: '#333' },
          focused && (isDark ? { borderColor: '#666' } : fieldSheet.boxFocused),
          !!error && fieldSheet.boxError,
        ]}
      >
        <TextInput
          ref={ref}
          style={[fieldSheet.input, isDark && { color: '#ffffff' }]}
          value={value}
          onChangeText={onChange}
          placeholder={placeholder}
          placeholderTextColor={isDark ? 'rgba(255,255,255,0.32)' : 'rgba(10,10,10,0.32)'}
          secureTextEntry={secure && !shown}
          keyboardType={keyboardType}
          autoCapitalize={autoCapitalize}
          autoCorrect={false}
          returnKeyType={returnKeyType}
          onFocus={handleFocus}
          onBlur={handleBlur}
          onSubmitEditing={onSubmit}
        />
        {secure && (
          <Pressable
            onPress={toggleShown}
            hitSlop={{ top: 16, bottom: 16, left: 16, right: 16 }}
            style={{ paddingLeft: 8 }}
          >
            <Feather name={shown ? 'eye-off' : 'eye'} size={19} color={isDark ? 'rgba(255,255,255,0.4)' : 'rgba(10,10,10,0.4)'} />
          </Pressable>
        )}
      </Pressable>
      {error ? <Text style={fieldSheet.err}>{error}</Text> : null}
    </View>
  );
});
Field.displayName = 'Field';

// ─── Screen ───────────────────────────────────────────────────────────────────

export default function AuthScreen() {
  const insets = useSafeAreaInsets();
  const router = useRouter();
  // Selectors, not the whole store: destructuring useAuthStore() subscribed this
  // screen to every auth state change and re-rendered the form each time.
  const login    = useAuthStore((st) => st.login);
  const register = useAuthStore((st) => st.register);
  const { isDark, colors } = useAppTheme();

  const [mode, setMode]           = useState<Mode>('login');
  const [loading, setLoading]     = useState(false);
  const [globalErr, setGlobalErr] = useState<string | null>(null);
  const [agreed, setAgreed]       = useState(false);
  const tabAnim = useRef(new Animated.Value(0)).current;

  const [loginEmail,    setLoginEmail]    = useState('');
  const [loginPassword, setLoginPassword] = useState('');
  const [firstName,     setFirstName]     = useState('');
  const [lastName,      setLastName]      = useState('');
  const [regEmail,      setRegEmail]      = useState('');
  const [phone,         setPhone]         = useState('');
  const [dob,           setDob]           = useState('');
  const [regPassword,   setRegPassword]   = useState('');
  const [confirmPass,   setConfirmPass]   = useState('');
  const [errors, setErrors] = useState<Record<string, string>>({});

  const loginPassRef = useRef<TextInput>(null);
  const lastNameRef  = useRef<TextInput>(null);
  const emailRef     = useRef<TextInput>(null);
  const phoneRef     = useRef<TextInput>(null);
  const dobRef       = useRef<TextInput>(null);
  const passRef      = useRef<TextInput>(null);
  const confirmRef   = useRef<TextInput>(null);


  // INPUT LAG: this used to build a brand-new errors object on every single
  // keystroke, which forced a second full re-render of the screen per character
  // even when there was no error to clear. Returning the same object when
  // nothing changes makes the update a no-op.
  const clearErr = useCallback((key: string) => {
    setErrors(p => (p[key] ? { ...p, [key]: '' } : p));
  }, []);

  const switchMode = useCallback((m: Mode) => {
    setMode(m); setGlobalErr(null); setErrors({});
    // Native driver: this animated `left` as a percentage before, which runs on
    // the JS thread and re-lays-out the track every frame. For ~400ms after each
    // tap the thread was too busy to register the next one.
    Animated.spring(tabAnim, { toValue: m === 'login' ? 0 : 1, useNativeDriver: true, tension: 140, friction: 11 }).start();
  }, [tabAnim]);

  const goLogin    = useCallback(() => switchMode('login'), [switchMode]);
  const goRegister = useCallback(() => switchMode('register'), [switchMode]);

  // ── Stable per-field handlers (required for React.memo on Field to bite) ──
  const onLoginEmail    = useCallback((v: string) => { setLoginEmail(v);    clearErr('loginEmail'); },    [clearErr]);
  const onLoginPassword = useCallback((v: string) => { setLoginPassword(v); clearErr('loginPassword'); }, [clearErr]);
  const onFirstName     = useCallback((v: string) => { setFirstName(v);     clearErr('firstName'); },     [clearErr]);
  const onLastName      = useCallback((v: string) => { setLastName(v);      clearErr('lastName'); },      [clearErr]);
  const onRegEmail      = useCallback((v: string) => { setRegEmail(v);      clearErr('regEmail'); },      [clearErr]);
  const onPhone         = useCallback((v: string) => { setPhone(v);         clearErr('phone'); },         [clearErr]);
  const onRegPassword   = useCallback((v: string) => { setRegPassword(v);   clearErr('regPassword'); },   [clearErr]);
  const onConfirmPass   = useCallback((v: string) => { setConfirmPass(v);   clearErr('confirmPass'); },   [clearErr]);

  const focusLoginPass = useCallback(() => loginPassRef.current?.focus(), []);
  const focusLastName  = useCallback(() => lastNameRef.current?.focus(),  []);
  const focusEmail     = useCallback(() => emailRef.current?.focus(),     []);
  const focusPhone     = useCallback(() => phoneRef.current?.focus(),     []);
  const focusDob       = useCallback(() => dobRef.current?.focus(),       []);
  const focusPass      = useCallback(() => passRef.current?.focus(),      []);
  const focusConfirm   = useCallback(() => confirmRef.current?.focus(),   []);

  const toggleAgreed = useCallback(() => { setAgreed(a => !a); clearErr('agreed'); }, [clearErr]);

  const onDob = useCallback((v: string) => { setDob(formatDob(v)); clearErr('dob'); }, [clearErr]);

  // Live password requirements shown under the Password field.
  const passChecks = [
    { label: 'At least 8 characters',              ok: regPassword.length >= 8 },
    { label: 'One capital letter (A–Z)',           ok: /[A-Z]/.test(regPassword) },
    { label: 'One number (0–9)',                   ok: /[0-9]/.test(regPassword) },
    { label: 'One special character (e.g. !@#$)',  ok: /[^A-Za-z0-9\s]/.test(regPassword) },
  ];
  const passAllOk = passChecks.every(c => c.ok);

  // Log in only lights up when both fields are plausibly filled.
  const loginFormValid = /\S+@\S+\.\S+/.test(loginEmail) && loginPassword.length > 0;

  // Sign up only lights up when every field is filled in correctly.
  const regFormValid =
    firstName.trim().length > 0 &&
    lastName.trim().length > 0 &&
    /\S+@\S+\.\S+/.test(regEmail) &&
    phone.replace(/\D/g, '').length >= 10 &&
    !!parseDobToISO(dob) &&
    passAllOk &&
    confirmPass === regPassword &&
    agreed;

  const handleLogin = useCallback(async () => {
    const errs: Record<string, string> = {};
    if (!loginEmail.trim())    errs.loginEmail    = 'Please enter your email';
    if (!loginPassword.trim()) errs.loginPassword = 'Please enter your password';
    if (Object.keys(errs).length) { setErrors(errs); return; }
    setLoading(true); setGlobalErr(null);
    try {
      await login(loginEmail.trim().toLowerCase(), loginPassword);
      router.back();
    } catch (e: any) {
      const msg = e?.response?.data?.error ?? '';
      setGlobalErr(msg.includes('credentials') ? 'Incorrect email or password.' : msg || 'Something went wrong.');
    } finally { setLoading(false); }
  }, [loginEmail, loginPassword, login, router]);

  const handleRegister = useCallback(async () => {
    const errs: Record<string, string> = {};
    if (!firstName.trim()) errs.firstName = 'Required';
    if (!lastName.trim())  errs.lastName  = 'Required';
    if (!regEmail.trim())  errs.regEmail  = 'Required';
    else if (!/\S+@\S+\.\S+/.test(regEmail)) errs.regEmail = 'Enter a valid email';
    if (!phone.trim())     errs.phone     = 'Required';
    else if (phone.replace(/\D/g, '').length < 10) errs.phone = 'Enter a valid number';
    if (!dob)              errs.dob       = 'Required';
    else if (!parseDobToISO(dob)) errs.dob = 'Use DD/MM/YYYY';
    if (!regPassword)      errs.regPassword = 'Required';
    else if (!passAllOk)   errs.regPassword = 'Password does not meet all requirements';
    if (confirmPass !== regPassword) errs.confirmPass = "Passwords don't match";
    if (!agreed)           errs.agreed    = 'You must agree to continue';
    if (Object.keys(errs).length) { setErrors(errs); return; }
    setLoading(true); setGlobalErr(null);
    try {
      await register({
        email: regEmail.trim().toLowerCase(), password: regPassword,
        first_name: firstName.trim(), last_name: lastName.trim(),
        phone_number: phone.trim(), date_of_birth: parseDobToISO(dob)!,
      });
      router.back();
    } catch (e: any) {
      const msg = e?.response?.data?.error ?? '';
      setGlobalErr(msg.toLowerCase().includes('email') ? 'An account with this email already exists.' : msg || 'Something went wrong.');
    } finally { setLoading(false); }
  }, [firstName, lastName, regEmail, phone, dob, regPassword, confirmPass, agreed, passAllOk, register, router]);


  // Tab indicator: measured once on layout, then moved with a native-driver
  // translateX instead of an animated `left` percentage.
  const [trackWidth, setTrackWidth] = useState(0);
  const onTrackLayout = useCallback((e: any) => {
    const w = e.nativeEvent.layout.width;
    setTrackWidth(prev => (Math.abs(prev - w) > 0.5 ? w : prev));
  }, []);
  const thumbWidth = trackWidth > 0 ? (trackWidth - 8) / 2 : 0;
  const indicatorX = tabAnim.interpolate({ inputRange: [0, 1], outputRange: [0, thumbWidth] });

  return (
   <View className={s.root} style={{ backgroundColor: isDark ? '#161618' : colors.background }}>
      <KeyboardAvoidingView style={{ flex: 1 }} behavior={Platform.OS === 'ios' ? 'padding' : 'height'}>
        {/* keyboardShouldPersistTaps: "handled" let the ScrollView swallow the
            first tap whenever the keyboard was up, so moving between fields or
            hitting a button took two taps. "always" delivers every tap straight
            to its target; keyboardDismissMode keeps the swipe-to-dismiss
            behaviour that "handled" was there for. */}
        <ScrollView className={s.scroll} contentContainerStyle={{ paddingBottom: insets.bottom + 48 }}
          keyboardShouldPersistTaps="always" keyboardDismissMode="on-drag"
          showsVerticalScrollIndicator={false}>

          {/* Close */}
          <TouchableOpacity className={s.closeBtn} style={{ marginTop: insets.top + 14, backgroundColor: isDark ? '#1c1c1c' : '#efefef' }}
            onPress={() => router.back()} hitSlop={{ top: 8, bottom: 8, left: 8, right: 8 }}>
             <Feather name="x" size={20} color={colors.text} />
          </TouchableOpacity>

          {/* Brand */}
          <View className={s.brand}>
            <BrandLogo size={96} />
            <Text className={s.brandTitle} style={{ color: colors.text }}>{mode === 'login' ? 'Welcome back' : 'Create account'}</Text>
            <Text className={s.brandSub} style={{ color: colors.subText }}>
              {mode === 'login' ? 'Sign in to access your tickets and bookings' : 'Join thousands of event-goers across South Africa'}
            </Text>
          </View>

          {/* Tab toggle */}
           <View className={s.tabTrack} style={{ backgroundColor: isDark ? '#1c1c1c' : '#ebebeb' }} onLayout={onTrackLayout}>
            <Animated.View
              pointerEvents="none"
              style={[
                sheet.tabThumb,
                {
                  left: 4,
                  width: thumbWidth,
                  transform: [{ translateX: indicatorX }],
                  backgroundColor: isDark ? '#2c2c2c' : '#fff',
                },
              ]}
            />
            <TouchableOpacity className={s.tab} onPress={goLogin} activeOpacity={0.75}>
              <Text className={s.tabLabel} style={{ color: mode === 'login' ? colors.text : colors.subText, fontWeight: mode === 'login' ? '700' : '600' }}>Log in</Text>
            </TouchableOpacity>
            <TouchableOpacity className={s.tab} onPress={goRegister} activeOpacity={0.75}>
              <Text className={s.tabLabel} style={{ color: mode === 'register' ? colors.text : colors.subText, fontWeight: mode === 'register' ? '700' : '600' }}>Sign up</Text>
            </TouchableOpacity>
          </View>

          {/* Global error */}
          {globalErr && (
            <View className={s.errBanner}>
              <Ionicons name="alert-circle-outline" size={15} color="#ff3c6e" />
              <Text className={s.errBannerText}>{globalErr}</Text>
            </View>
          )}

          {/* ── LOGIN ── */}
          {mode === 'login' && (
            <View className={s.form}>
              <Field placeholder="Email address" value={loginEmail}
                onChange={onLoginEmail}
                keyboardType="email-address" error={errors.loginEmail}
                returnKeyType="next" onSubmit={focusLoginPass} isDark={isDark} />
              <Field placeholder="Password" value={loginPassword}
                onChange={onLoginPassword}
                secure inputRef={loginPassRef} error={errors.loginPassword}
                returnKeyType="done" onSubmit={handleLogin} isDark={isDark} />

              <TouchableOpacity className={s.forgotWrap} onPress={() => router.push({ pathname: '/auth/forgot', params: { email: loginEmail.trim() } })}>
                <Text className={s.forgotText} style={{ color: colors.text }}>Forgot password?</Text>
              </TouchableOpacity>

               <TouchableOpacity
                style={[
                  sheet.cta,
                  loading && sheet.ctaDisabled,
                  { backgroundColor: loginFormValid ? (isDark ? '#ffffff' : '#0a0a0a') : (isDark ? '#232326' : '#e2e2e6') },
                ]}
                onPress={handleLogin} activeOpacity={0.85} disabled={loading || !loginFormValid}>
                {loading
                  ? <ActivityIndicator color={isDark ? '#000' : '#fff'} size="small" />
                  : <Text className={s.ctaText} style={{ color: loginFormValid ? (isDark ? '#0a0a0a' : '#ffffff') : (isDark ? 'rgba(255,255,255,0.35)' : 'rgba(10,10,10,0.35)') }}>Log in</Text>}
              </TouchableOpacity>


              <TouchableOpacity className={s.switchRow} onPress={goRegister}>
                <Text className={s.switchText} style={{ color: colors.subText }}>Don't have an account?{'  '}<Text className={s.switchLink}>Sign up</Text></Text>
              </TouchableOpacity>
            </View>
          )}

           
          {/* ── REGISTER ── */}
          {mode === 'register' && (
            <View className={s.form}>
              <View className={s.nameRow}>
                <View style={{ flex: 1 }}>
                  <Field placeholder="First name" value={firstName}
                  onChange={onFirstName}
                  autoCapitalize="words" error={errors.firstName}
                  returnKeyType="next" onSubmit={focusLastName} isDark={isDark} />
                </View>
                <View style={{ flex: 1 }}>
                  <Field placeholder="Last name" value={lastName}
                  onChange={onLastName}
                  autoCapitalize="words" inputRef={lastNameRef} error={errors.lastName}
                  returnKeyType="next" onSubmit={focusEmail} isDark={isDark} />
                </View>
              </View>

              <Field placeholder="Email address" value={regEmail}
              onChange={onRegEmail}
              keyboardType="email-address" inputRef={emailRef} error={errors.regEmail}
              returnKeyType="next" onSubmit={focusPhone} isDark={isDark} />

              <View className={s.phoneRow}>
                <View style={[sheet.dialCode, isDark && { backgroundColor: '#1c1c21', borderColor: '#2e2e35' }]}>
                  <Text className={s.dialFlag}>🇿🇦</Text>
                  <Text className={s.dialNum} style={{ color: colors.text }}>+27</Text>
                </View>
                <View style={{ flex: 1 }}>
                  <Field placeholder="Phone number" value={phone}
                  onChange={onPhone}
                  keyboardType="phone-pad" inputRef={phoneRef} error={errors.phone}
                  returnKeyType="next" onSubmit={focusDob} isDark={isDark} />

                </View>
              </View>

              <Field placeholder="Date of birth (DD/MM/YYYY)" value={dob}
                onChange={onDob}
                keyboardType="numeric" inputRef={dobRef} error={errors.dob}
                returnKeyType="next" onSubmit={focusPass} isDark={isDark} />

              <Field placeholder="Password" value={regPassword}
              onChange={onRegPassword}
              secure inputRef={passRef} error={errors.regPassword}
              returnKeyType="next" onSubmit={focusConfirm} isDark={isDark} />

              {/* Live password requirements */}
              <View style={{ marginTop: -2, marginBottom: 12, gap: 6, paddingHorizontal: 4 }}>
                {passChecks.map((c) => (
                  <View key={c.label} style={{ flexDirection: 'row', alignItems: 'center', gap: 7 }}>
                    <CheckCircle
                      size={15}
                      color={c.ok ? '#22c55e' : (isDark ? 'rgba(255,255,255,0.28)' : 'rgba(10,10,10,0.25)')}
                      weight={c.ok ? 'fill' : 'regular'}
                    />
                    <Text style={{ fontSize: 12.5, color: c.ok ? '#22c55e' : colors.subText }}>{c.label}</Text>
                  </View>
                ))}
              </View>

              <Field placeholder="Confirm password" value={confirmPass}
                onChange={onConfirmPass}
                secure inputRef={confirmRef} error={errors.confirmPass}
                returnKeyType="done" onSubmit={handleRegister} isDark={isDark} />

              {/* Terms */}
              <TouchableOpacity className={s.termsRow}
                onPress={toggleAgreed} activeOpacity={0.75}>
                <View style={[
                  sheet.checkbox,
                  { borderColor: isDark ? '#4a4a52' : '#ccc' },
                  agreed && { backgroundColor: colors.text, borderColor: colors.text },
                ]}>
                  {agreed && <Ionicons name="checkmark" size={13} color={isDark ? '#000' : '#fff'} />}
                </View>
                <Text className={s.termsText} style={{ color: colors.subText }}>
                  {/* The two document names were styled as links but had no
                      handler, so tapping them only toggled the checkbox. A user
                      is being asked to agree to terms they had no way to read. */}
                  By checking this box, you agree to our{' '}
                  <Text
                    className={s.termsLink}
                    style={{ color: colors.text }}
                    onPress={(e) => { e.stopPropagation(); openLegal('terms'); }}
                    suppressHighlighting
                  >
                    Terms of Use
                  </Text>
                  {' '}and acknowledge having read our{' '}
                  <Text
                    className={s.termsLink}
                    style={{ color: colors.text }}
                    onPress={(e) => { e.stopPropagation(); openLegal('privacy'); }}
                    suppressHighlighting
                  >
                    Privacy Policy
                  </Text>.
                </Text>
              </TouchableOpacity>
              {errors.agreed ? <Text className={s.termsErr}>{errors.agreed}</Text> : null}

              <TouchableOpacity
                style={[
                  sheet.cta,
                  loading && sheet.ctaDisabled,
                  { backgroundColor: regFormValid ? (isDark ? '#ffffff' : '#0a0a0a') : (isDark ? '#232326' : '#e2e2e6') },
                ]}
                onPress={handleRegister} activeOpacity={0.85} disabled={loading || !regFormValid}>
                {loading
                  ? <ActivityIndicator color={isDark ? '#000' : '#fff'} size="small" />
                  : <Text className={s.ctaText} style={{ color: regFormValid ? (isDark ? '#0a0a0a' : '#ffffff') : (isDark ? 'rgba(255,255,255,0.35)' : 'rgba(10,10,10,0.35)') }}>Sign up</Text>}
              </TouchableOpacity>


              <TouchableOpacity className={s.switchRow} onPress={goLogin}>
                <Text className={s.switchText} style={{ color: colors.subText }}>Already have an account?{'  '}<Text className={s.switchLink}>Log in</Text></Text>
              </TouchableOpacity>
            </View>
          )}     
        </ScrollView>
      </KeyboardAvoidingView>
    </View>
  );
}