
import express from 'express';
import cors from 'cors';
import dotenv from 'dotenv';

const app = express();
dotenv.config();

app.use(cors());

const PORT = process.env.PORT || 6050;

app.listen(PORT, ()=>{
    try {
        console.log("Server is running on " + PORT);
    } catch (err) {
        console.log(err);
    }
})