import React, { useState, useRef, useCallback } from 'react';
import {
  View, Text, TextInput, TouchableOpacity, ScrollView,
  KeyboardAvoidingView, Platform, ActivityIndicator, Animated,
} from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { Feather, Ionicons } from '@expo/vector-icons';
import { useRouter } from 'expo-router';
import { useAuthStore } from '../../store/authStore';
import { s, sheet, fieldSheet } from '../../styles/auth';
import { useAppTheme } from '../../theme/colors';        // ADD
import { useThemeStore, ThemeMode } from '../../store/themeStore'; // ADD

type Mode = 'login' | 'register';

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

const Field = ({ placeholder, value, onChange, secure, keyboardType = 'default',
  autoCapitalize = 'none', error, returnKeyType = 'next', onSubmit, inputRef,
  isDark }: FieldProps) => {
  const [focused, setFocused] = useState(false);
  const [shown, setShown]     = useState(false);
  return (
    <View style={fieldSheet.wrap}>
      <View style={[
        fieldSheet.box,
        isDark && { backgroundColor: '#1c1c1c', borderColor: '#333' },
        focused && (isDark ? { borderColor: '#666' } : fieldSheet.boxFocused),
        !!error && fieldSheet.boxError,
      ]}>
        <TextInput
          ref={inputRef}
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
          onFocus={() => setFocused(true)}
          onBlur={() => setFocused(false)}
          onSubmitEditing={onSubmit}
        />
        {secure && (
          <TouchableOpacity onPress={() => setShown(p => !p)} hitSlop={{ top: 8, bottom: 8, left: 8, right: 8 }}>
           <Feather name={shown ? 'eye-off' : 'eye'} size={19} color={isDark ? 'rgba(255,255,255,0.4)' : 'rgba(10,10,10,0.4)'} />
          </TouchableOpacity>
        )}
      </View>
      {error ? <Text style={fieldSheet.err}>{error}</Text> : null}
    </View>
  );
};

// ─── Screen ───────────────────────────────────────────────────────────────────

export default function AuthScreen() {
  const insets = useSafeAreaInsets();
  const router = useRouter();
  const { login, register } = useAuthStore();
  const { isDark, colors } = useAppTheme();
  const { mode: themeMode, setMode: setThemeMode } = useThemeStore();

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
  const phoneRef     = useRef<TextInput>(null);
  const dobRef       = useRef<TextInput>(null);
  const passRef      = useRef<TextInput>(null);
  const confirmRef   = useRef<TextInput>(null);

  const themeModeLabel: Record<ThemeMode, string> = { auto: 'Auto', light: 'Light', dark: 'Dark' };
  const cycleTheme = () => {
    const next: Record<ThemeMode, ThemeMode> = { auto: 'light', light: 'dark', dark: 'auto' };
    setThemeMode(next[themeMode]);
  };

  const clearErr = (key: string) => setErrors(p => ({ ...p, [key]: '' }));

  const switchMode = (m: Mode) => {
    setMode(m); setGlobalErr(null); setErrors({});
    Animated.spring(tabAnim, { toValue: m === 'login' ? 0 : 1, useNativeDriver: false, tension: 140, friction: 11 }).start();
  };

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
    else if (regPassword.length < 8) errs.regPassword = 'Min. 8 characters';
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
  }, [firstName, lastName, regEmail, phone, dob, regPassword, confirmPass, agreed, register, router]);

  const indicatorLeft = tabAnim.interpolate({ inputRange: [0, 1], outputRange: ['2%', '51%'] });

  return (
   <View className={s.root} style={{ backgroundColor: colors.background }}>
      <KeyboardAvoidingView style={{ flex: 1 }} behavior={Platform.OS === 'ios' ? 'padding' : 'height'}>
        <ScrollView className={s.scroll} contentContainerStyle={{ paddingBottom: insets.bottom + 48 }}
          keyboardShouldPersistTaps="handled" showsVerticalScrollIndicator={false}>

          {/* Close */}
          <TouchableOpacity className={s.closeBtn} style={{ marginTop: insets.top + 14, backgroundColor: isDark ? '#1c1c1c' : '#efefef' }}
            onPress={() => router.back()} hitSlop={{ top: 8, bottom: 8, left: 8, right: 8 }}>
             <Feather name="x" size={20} color={colors.text} />
          </TouchableOpacity>

          {/* Brand */}
          <View className={s.brand}>
            <View className={s.logoMark} style={sheet.logoShadow}>
              <Ionicons name="ticket" size={26} color="#fff" />
            </View>
            <Text className={s.brandTitle} style={{ color: colors.text }}>{mode === 'login' ? 'Welcome back' : 'Create account'}</Text>
            <Text className={s.brandSub} style={{ color: colors.subText }}>
              {mode === 'login' ? 'Sign in to access your tickets and bookings' : 'Join thousands of event-goers across South Africa'}
            </Text>
          </View>

          {/* Tab toggle */}
           <View className={s.tabTrack} style={{ backgroundColor: isDark ? '#1c1c1c' : '#ebebeb' }}>
            <Animated.View style={[sheet.tabThumb, { left: indicatorLeft, backgroundColor: isDark ? '#2c2c2c' : '#fff' }]} />
            <TouchableOpacity className={s.tab} onPress={() => switchMode('login')} activeOpacity={0.75}>
              <Text className={s.tabLabel} style={{ color: mode === 'login' ? colors.text : colors.subText, fontWeight: mode === 'login' ? '700' : '600' }}>Log in</Text>
            </TouchableOpacity>
            <TouchableOpacity className={s.tab} onPress={() => switchMode('register')} activeOpacity={0.75}>
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
                onChange={v => { setLoginEmail(v); clearErr('loginEmail'); }}
                keyboardType="email-address" error={errors.loginEmail}
                returnKeyType="next" onSubmit={() => loginPassRef.current?.focus()} isDark={isDark} />
              <Field placeholder="Password" value={loginPassword}
                onChange={v => { setLoginPassword(v); clearErr('loginPassword'); }}
                secure inputRef={loginPassRef} error={errors.loginPassword}
                returnKeyType="done" onSubmit={handleLogin} isDark={isDark} />

              <TouchableOpacity className={s.forgotWrap} onPress={() => {}}>
                <Text className={s.forgotText}>Forgot password?</Text>
              </TouchableOpacity>

               <TouchableOpacity style={[sheet.cta, loading && sheet.ctaDisabled, { backgroundColor: isDark ? '#ffffff' : '#0a0a0a' }]}
                onPress={handleLogin} activeOpacity={0.85} disabled={loading}>
                {loading ? <ActivityIndicator color={isDark ? '#000' : '#fff'} size="small" /> : <Text className={s.ctaText} style={{ color: isDark ? '#0a0a0a' : '#ffffff' }}>Log in</Text>}
              </TouchableOpacity>

              {/* Theme toggle */}
              <TouchableOpacity
                onPress={cycleTheme}
                style={{
                  flexDirection: 'row', alignItems: 'center', justifyContent: 'center',
                  gap: 8, paddingVertical: 12, borderRadius: 12,
                  backgroundColor: isDark ? '#1c1c1c' : '#f0f0f0', marginTop: 2,
                }}
              >
                <Feather name={isDark ? 'moon' : 'sun'} size={15} color={colors.text} />
                <Text style={{ color: colors.text, fontSize: 13, fontWeight: '600' }}>
                  {themeModeLabel[themeMode]} mode
                </Text>
              </TouchableOpacity>

              <TouchableOpacity className={s.switchRow} onPress={() => switchMode('register')}>
                <Text className={s.switchText} style={{ color: colors.subText }}>Don't have an account?{'  '}<Text className={s.switchLink}>Sign up</Text></Text>
              </TouchableOpacity>
            </View>
          )}

           
          {/* ── REGISTER ── */}
          {mode === 'register' && (
            <View className={s.form}>
              <Field placeholder="Email address" value={regEmail}
              onChange={v => { setRegEmail(v); clearErr('regEmail'); }}
              keyboardType="email-address" error={errors.regEmail}
              returnKeyType="next" onSubmit={() => lastNameRef.current?.focus()} isDark={isDark} />

              <View className={s.nameRow}>
                <View style={{ flex: 1 }}>
                  <Field placeholder="First name" value={firstName}
                  onChange={v => { setFirstName(v); clearErr('firstName'); }}
                  autoCapitalize="words" error={errors.firstName}
                  returnKeyType="next" onSubmit={() => lastNameRef.current?.focus()} isDark={isDark} />
                </View>
                <View style={{ flex: 1 }}>
                  <Field placeholder="Last name" value={lastName}
                  onChange={v => { setLastName(v); clearErr('lastName'); }}
                  autoCapitalize="words" inputRef={lastNameRef} error={errors.lastName}
                  returnKeyType="next" onSubmit={() => phoneRef.current?.focus()} isDark={isDark} />
                </View>
              </View>

              <Field placeholder="Password" value={regPassword}
              onChange={v => { setRegPassword(v); clearErr('regPassword'); }}
              secure inputRef={passRef} error={errors.regPassword}
              returnKeyType="next" onSubmit={() => phoneRef.current?.focus()} isDark={isDark} />

              <View className={s.phoneRow}>
                <View style={sheet.dialCode}>
                  <Text className={s.dialFlag}>🇿🇦</Text>
                  <Text className={s.dialNum} style={{ color: colors.text }}>+27</Text>
                </View>
                <View style={{ flex: 1 }}>
                  <Field placeholder="Phone number" value={phone}
                  onChange={v => { setPhone(v); clearErr('phone'); }}
                  keyboardType="phone-pad" inputRef={phoneRef} error={errors.phone}
                  returnKeyType="next" onSubmit={() => dobRef.current?.focus()} isDark={isDark} />

                </View>
              </View>

              <Field placeholder="Date of birth (DD/MM/YYYY)" value={dob}
                onChange={v => { setDob(formatDob(v)); clearErr('dob'); }}
                keyboardType="numeric" inputRef={dobRef} error={errors.dob}
                returnKeyType="next" onSubmit={() => confirmRef.current?.focus()} isDark={isDark} />

              <Field placeholder="Confirm password" value={confirmPass}
                onChange={v => { setConfirmPass(v); clearErr('confirmPass'); }}
                secure inputRef={confirmRef} error={errors.confirmPass}
                returnKeyType="done" onSubmit={handleRegister} isDark={isDark} />

              {/* Terms */}
              <TouchableOpacity className={s.termsRow}
                onPress={() => { setAgreed(a => !a); clearErr('agreed'); }} activeOpacity={0.75}>
                <View style={[sheet.checkbox, agreed && sheet.checkboxChecked]}>
                  {agreed && <Ionicons name="checkmark" size={13} color="#fff" />}
                </View>
                <Text className={s.termsText}>
                  By checking this box, you agree to our <Text className={s.termsLink}>Terms of Use</Text>
                  {' '}and acknowledge having read our <Text className={s.termsLink}>Privacy Policy</Text>.
                </Text>
              </TouchableOpacity>
              {errors.agreed ? <Text className={s.termsErr}>{errors.agreed}</Text> : null}

              <TouchableOpacity style={[sheet.cta, loading && sheet.ctaDisabled, { backgroundColor: isDark ? '#ffffff' : '#0a0a0a' }]}
                onPress={handleRegister} activeOpacity={0.85} disabled={loading}>
                {loading ? <ActivityIndicator color={isDark ? '#000' : '#fff'} size="small" /> : <Text className={s.ctaText} style={{ color: isDark ? '#0a0a0a' : '#ffffff' }}>Sign up</Text>}
              </TouchableOpacity>

              {/* Theme toggle */}
              <TouchableOpacity
                onPress={cycleTheme}
                style={{
                  flexDirection: 'row', alignItems: 'center', justifyContent: 'center',
                  gap: 8, paddingVertical: 12, borderRadius: 12,
                  backgroundColor: isDark ? '#1c1c1c' : '#f0f0f0', marginTop: 2,
                }}
              >
                <Feather name={isDark ? 'moon' : 'sun'} size={15} color={colors.text} />
                <Text style={{ color: colors.text, fontSize: 13, fontWeight: '600' }}>
                  {themeModeLabel[themeMode]} mode
                </Text>
              </TouchableOpacity>

              <TouchableOpacity className={s.switchRow} onPress={() => switchMode('login')}>
                <Text className={s.switchText} style={{ color: colors.subText }}>Already have an account?{'  '}<Text className={s.switchLink}>Log in</Text></Text>
              </TouchableOpacity>
            </View>
          )}     
        </ScrollView>
      </KeyboardAvoidingView>
    </View>
  );
}