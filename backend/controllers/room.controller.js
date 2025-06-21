import { Room } from '../model/Room.model.js';
const deleteRoom = async (req, res) => {
    try {
        const { roomId } = req.params;
        await Room.deleteOne({ roomId });
        req.io.to(roomId).emit('roomDeleted');
        res.status(200).json({ success: true });
    } catch (error) {
        console.error('Error deleting room:', error);
        res.status(500).json({ success: false });
    }
};

export { deleteRoom };