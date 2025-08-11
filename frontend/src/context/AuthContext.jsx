import { createContext, useContext, useEffect, useState } from "react";
import { supabase } from "../utils/supabase";

const AuthContext = createContext();

export function AuthProvider({ children }) {
    const [userId, setUserId] = useState(null);
    const [email, setEmail] = useState(null);

    // Room state
    const [users, setUsers] = useState([]);
    const [adminId, setAdminId] = useState(null);
    const [isAdmin, setIsAdmin] = useState(false);

    useEffect(() => {
        const getUser = async () => {
            const { data: { session } } = await supabase.auth.getSession();
            if (session?.user) {
                setUserId(session.user.id);
                setEmail(session.user.email);
            }
        };

        getUser();

        const { data: listener } = supabase.auth.onAuthStateChange((_event, session) => {
            if (session?.user) {
                setUserId(session.user.id);
                setEmail(session.user.email);
            } else {
                setUserId(null);
                setEmail(null);
            }
        });

        return () => {
            listener.subscription.unsubscribe();
        };
    }, []);

    return (
        <AuthContext.Provider value={{
            userId,
            email,
            users,
            adminId,
            setUsers,
            setAdminId,
            isAdmin, 
            setIsAdmin
        }}>
            {children}
        </AuthContext.Provider>
    );
}

export function useAuth() {
    return useContext(AuthContext);
}
