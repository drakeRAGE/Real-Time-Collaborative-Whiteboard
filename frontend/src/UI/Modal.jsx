import React from 'react'

const Modal = ({ handleClick, setShowModal, name, message }) => {
    return (
        <div>
            <div style={{
                position: 'fixed',
                top: 0,
                left: 0,
                right: 0,
                bottom: 0,
                backgroundColor: 'rgba(0,0,0,0.5)',
                display: 'flex',
                justifyContent: 'center',
                alignItems: 'center',
                zIndex: 1000
            }}>
                <div style={{
                    backgroundColor: 'white',
                    padding: '2rem',
                    borderRadius: '8px',
                    maxWidth: '400px'
                }}>
                    <h2 style={{ marginBottom: '1rem', color: 'black' }}>{name}</h2>
                    <p className='text-black'>{message}</p>
                    <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '1rem', marginTop: '1.5rem' }}>
                        <button
                            onClick={() => setShowModal(false)}
                            style={{
                                padding: '0.5rem 1rem',
                                background: '#e5e7eb',
                                border: 'none',
                                borderRadius: '4px',
                                cursor: 'pointer'
                            }}
                        >
                            Cancel
                        </button>
                        <button
                            onClick={() => {
                                handleClick();
                                setShowModal(false);
                            }}

                            style={{
                                padding: '0.5rem 1rem',
                                background: '#ef4444',
                                color: 'white',
                                border: 'none',
                                borderRadius: '4px',
                                cursor: 'pointer'
                            }}
                        >
                            {name}
                        </button>
                    </div>
                </div>
            </div>
        </div>
    )
}

export default Modal