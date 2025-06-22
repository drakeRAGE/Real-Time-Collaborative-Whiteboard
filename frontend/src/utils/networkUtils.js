export const CheckOline = () => {
    if (!navigator.onLine) {
        alert("Network error: Please check your internet connection");
        return;
    }
    return true; // true is the user is online
}

export const updateStatus = (status, setIsOnline, setShowBackOnline, ONLINE_HASH, OFFLINE_HASH) => {
    setIsOnline(status);
    localStorage.setItem("network", status ? ONLINE_HASH : OFFLINE_HASH);

    if (status) {
        setShowBackOnline(true);
        setTimeout(() => setShowBackOnline(false), 3000); // hide after 3s
    }
};